package tfjson

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"testing"
)

func TestReconcileManagedDriftSecretsAndDefaults(t *testing.T) {
	prior := types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{"host": types.StringType, "password": types.StringType, "port": types.Int64Type}, map[string]attr.Value{"host": types.StringValue("old"), "password": types.StringValue("private"), "port": types.Int64Value(22)}))
	same, err := Reconcile(prior, []byte(`{"host":"old","password":"****","port":22,"timeout":30}`), []string{"password"})
	if err != nil || !same.Equal(prior) {
		t.Fatalf("equivalent refresh must preserve types: %v %v", same, err)
	}
	got, err := Reconcile(prior, []byte(`{"host":"new","password":"****","port":23,"timeout":30}`), []string{"password"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := ToRawJSON(got)
	if string(raw) != `{"host":"new","password":"private","port":23}` {
		t.Fatalf("drift lost: %s", raw)
	}
}
func TestReconcileImportAndLiteralMasks(t *testing.T) {
	got, err := Reconcile(types.DynamicNull(), []byte(`{"password":"***","username":"****","host":"readable"}`), []string{"password"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := ToRawJSON(got)
	if string(raw) != `{"host":"readable","password":null,"username":"****"}` {
		t.Fatalf("unsafe import: %s", raw)
	}
}
func TestReconcileRemoteClear(t *testing.T) {
	p, _ := FromRawJSON([]byte(`{"temperature":0.2}`))
	got, err := Reconcile(p, []byte(`null`), nil)
	if err != nil || !got.IsNull() {
		t.Fatalf("remote clear lost: %v %v", got, err)
	}
}

func TestReconcileUnknownHydrates(t *testing.T) {
	got, err := Reconcile(types.DynamicUnknown(), []byte(`{"host":"new"}`), nil)
	if err != nil || got.IsUnknown() {
		t.Fatalf("hydrate unknown: %v %v", got, err)
	}
}
