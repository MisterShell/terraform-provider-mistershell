package canaryextras

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestOrderChecksCompleteSetAndNeverDeletes(t *testing.T) {
	for _, factory := range []func() resource.Resource{NewCanaryRuleOrderResource, NewSyslogRuleOrderResource} {
		r := factory().(*OrderResource)
		t.Run(r.name, func(t *testing.T) {
			ids := []int64{1, 2}
			writes := 0
			conflict := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Authorization") != "Bearer key" {
					t.Error("missing auth")
				}
				switch req.Method {
				case "GET":
					rows := []map[string]int64{}
					for i, id := range ids {
						rows = append(rows, map[string]int64{"id": id, "position": int64(i)})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": rows})
				case "PUT":
					writes++
					if req.URL.Path != r.orderPath {
						t.Error("wrong order endpoint")
					}
					if conflict {
						w.WriteHeader(409)
						_, _ = w.Write([]byte(`{"success":false,"message":"changed"}`))
						return
					}
					var body struct {
						IDs []int64 `json:"ordered_ids"`
					}
					_ = json.NewDecoder(req.Body).Decode(&body)
					ids = body.IDs
					_, _ = w.Write([]byte(`{"success":true,"data":null}`))
				default:
					t.Errorf("unexpected destructive method %s", req.Method)
				}
			}))
			defer server.Close()
			r.client = client.NewClient(server.URL, "key", server.Client())
			ctx := context.Background()
			for _, bad := range [][]int64{{1}, {1, 1}, {1, 3}} {
				v, _ := types.ListValueFrom(ctx, types.Int64Type, bad)
				if err := r.apply(ctx, OrderModel{OrderedIDs: v}); err == nil {
					t.Fatalf("accepted incomplete/duplicate/unknown order %v", bad)
				}
			}
			if writes != 0 {
				t.Fatal("invalid ordering reached API mutation")
			}
			v, _ := types.ListValueFrom(ctx, types.Int64Type, []int64{2, 1})
			if err := r.apply(ctx, OrderModel{OrderedIDs: v}); err != nil {
				t.Fatal(err)
			}
			if writes != 1 || ids[0] != 2 {
				t.Fatal("order did not update")
			}
			conflict = true
			if err := r.apply(ctx, OrderModel{OrderedIDs: v}); err == nil {
				t.Fatal("409 swallowed")
			}
			if writes != 2 {
				t.Fatal("conflicting write retried")
			}
			r.Delete(ctx, resource.DeleteRequest{}, &resource.DeleteResponse{})
			if writes != 2 {
				t.Fatal("destroy changed policy")
			}
		})
	}
}
func TestOverrideRestoresOriginalFlagAndRejectsCustom(t *testing.T) {
	ctx := context.Background()
	enabled := false
	builtin := true
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			var seed any
			if builtin {
				seed = "canary:ssh"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 1, "seed_key": seed, "enabled": enabled}})
		case "PUT":
			var body map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 1 {
				t.Errorf("override wrote unrelated fields: %v", body)
			}
			enabled = body["enabled"]
			writes++
			_, _ = w.Write([]byte(`{"success":true,"data":null}`))
		default:
			t.Fatalf("override must never delete builtin: %s", r.Method)
		}
	}))
	defer server.Close()
	r := &ServiceOverrideResource{client: client.NewClient(server.URL, "key", server.Client())}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	state := tfsdk.State{Schema: sr.Schema}
	d := state.Set(ctx, ServiceOverrideModel{ID: types.StringNull(), ServiceID: types.Int64Value(1), Enabled: types.BoolValue(true), OriginalEnabled: types.BoolUnknown()})
	if d.HasError() {
		t.Fatal(d)
	}
	var created resource.CreateResponse
	created.State.Schema = sr.Schema
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	if !enabled || writes != 1 {
		t.Fatal("override not applied")
	}
	var restored resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &restored)
	if restored.Diagnostics.HasError() {
		t.Fatal(restored.Diagnostics)
	}
	if enabled || writes != 2 {
		t.Fatal("destroy did not restore baseline")
	}
	builtin = false
	if err := r.apply(ctx, ServiceOverrideModel{ServiceID: types.Int64Value(1), Enabled: types.BoolValue(true)}); err == nil {
		t.Fatal("custom service accepted")
	}
	if writes != 2 {
		t.Fatal("custom service mutated")
	}
}

func TestOverrideRetainsBaselineWhenPostWriteReadFails(t *testing.T) {
	ctx := context.Background()
	written := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			written = true
			_, _ = w.Write([]byte(`{"success":true,"data":null}`))
			return
		}
		if written {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"success":false,"message":"temporarily unavailable"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":1,"seed_key":"canary:ssh","enabled":false}}`))
	}))
	defer server.Close()
	r := &ServiceOverrideResource{client: client.NewClient(server.URL, "key", server.Client())}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	s := tfsdk.State{Schema: sr.Schema}
	if d := s.Set(ctx, ServiceOverrideModel{ID: types.StringNull(), ServiceID: types.Int64Value(1), Enabled: types.BoolValue(true), OriginalEnabled: types.BoolUnknown()}); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: sr.Schema, Raw: s.Raw}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("verification outage ignored")
	}
	var m ServiceOverrideModel
	if d := resp.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.ID.ValueString() != "1" || m.OriginalEnabled.IsUnknown() || m.OriginalEnabled.IsNull() || m.OriginalEnabled.ValueBool() {
		t.Fatal("original flag lost after successful write")
	}
}

func TestCanaryOrderingReadsAllPagesBeforeWriting(t *testing.T) {
	writes := 0
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes++
			return
		}
		pages++
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":1,"position":10}],"meta":{"page":1,"pages":2,"total":2,"size":1}}`))
		} else {
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":2,"position":20}],"meta":{"page":2,"pages":2,"total":2,"size":1}}`))
		}
	}))
	defer server.Close()
	r := NewCanaryRuleOrderResource().(*OrderResource)
	r.client = client.NewClient(server.URL, "key", server.Client())
	ctx := context.Background()
	ids, _ := types.ListValueFrom(ctx, types.Int64Type, []int64{1})
	if err := r.apply(ctx, OrderModel{OrderedIDs: ids}); err == nil {
		t.Fatal("omitted second-page rule accepted")
	}
	if writes != 0 || pages != 2 {
		t.Fatalf("must read full set before write: pages=%d writes=%d", pages, writes)
	}
}
