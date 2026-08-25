package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testDiscoveryToken = "opaque-discovery-token-at-least-32-bytes"

func TestDiscoverNetworkResourceAutomaticGenericSSHFallback(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		switch call {
		case 1:
			assertOnboardingRequest(t, r, http.MethodPost, "/api/v1/resources/test/discovery")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["engine_id"] != "ssh" || body["credential_id"] != float64(7) || body["location_id"] != float64(3) {
				t.Fatalf("discovery body = %#v", body)
			}
			if _, exists := body["expected_type_id"]; exists {
				t.Fatalf("automatic discovery sent expected_type_id: %#v", body)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"started","data":{"task_id":41,"status":"queued","discovery_token":%q,"discovery_proof_ttl_seconds":14400}}`, testDiscoveryToken)))
		case 2:
			assertOnboardingRequest(t, r, http.MethodGet, "/api/v1/worker-tasks/41")
			_, _ = w.Write([]byte(`{"success":true,"message":"done","data":{"id":41,"status":"success","result":{"status":"verified","discovery":{"status":"fallback","engine_id":"ssh","type_id":"generic_ssh"},"external_id":null},"result_success":true,"result_message":"Connected to generic_ssh"}}`))
		case 3:
			assertOnboardingRequest(t, r, http.MethodPost, "/api/v1/resources/test/discovery/proof-status")
			var body DiscoveryProofStatusInput
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.DiscoveryToken != testDiscoveryToken {
				t.Fatalf("proof token = %q", body.DiscoveryToken)
			}
			_, _ = w.Write([]byte(`{"success":true,"message":"ready","data":{"state":"ready"}}`))
		default:
			t.Fatalf("unexpected request %d: %s %s", call, r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newOnboardingTestClient(srv)
	got, err := c.DiscoverNetworkResource(context.Background(), DiscoveryTestInput{
		EngineID:      "ssh",
		CredentialID:  7,
		ConnectorData: json.RawMessage(`{"host":"192.0.2.10","port":22}`),
		LocationID:    3,
	})
	if err != nil {
		t.Fatalf("DiscoverNetworkResource returned an error: %v", err)
	}
	if got.TaskID != 41 || got.EngineID != "ssh" || got.ResourceType != "generic_ssh" || got.DiscoveryToken != testDiscoveryToken {
		t.Fatalf("result = %#v", got)
	}
	if call != 3 {
		t.Fatalf("request count = %d, want 3", call)
	}
}

func TestDiscoverNetworkResourceExplicitType(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		switch call {
		case 1:
			var body DiscoveryTestInput
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.ExpectedTypeID == nil || *body.ExpectedTypeID != "cisco_ios" {
				t.Fatalf("expected type = %#v", body.ExpectedTypeID)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"started","data":{"task_id":42,"status":"queued","discovery_token":%q}}`, testDiscoveryToken)))
		case 2:
			_, _ = w.Write([]byte(`{"success":true,"message":"done","data":{"id":42,"status":"success","result":{"discovery":{"status":"matched","engine_id":"ssh","type_id":"cisco_ios"}},"result_success":true}}`))
		case 3:
			_, _ = w.Write([]byte(`{"success":true,"message":"ready","data":{"state":"ready"}}`))
		default:
			t.Fatalf("unexpected request %d", call)
		}
	}))
	defer srv.Close()

	expected := "cisco_ios"
	c := newOnboardingTestClient(srv)
	got, err := c.DiscoverNetworkResource(context.Background(), DiscoveryTestInput{
		EngineID: "ssh", ExpectedTypeID: &expected, CredentialID: 7,
		ConnectorData: json.RawMessage(`{"host":"192.0.2.10"}`), LocationID: 3,
	})
	if err != nil || got.ResourceType != expected {
		t.Fatalf("result=%#v err=%v", got, err)
	}
}

func TestDiscoverNetworkResourcePollsTaskAndProof(t *testing.T) {
	taskReads := 0
	proofReads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/resources/test/discovery":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"started","data":{"task_id":43,"status":"queued","discovery_token":%q}}`, testDiscoveryToken)))
		case "/api/v1/worker-tasks/43":
			taskReads++
			if taskReads == 1 {
				_, _ = w.Write([]byte(`{"success":true,"message":"running","data":{"id":43,"status":"running","result_success":null}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"message":"done","data":{"id":43,"status":"success","result":{"discovery":{"status":"matched","engine_id":"ssh","type_id":"linux"}},"result_success":true}}`))
		case "/api/v1/resources/test/discovery/proof-status":
			proofReads++
			state := "pending"
			if proofReads == 2 {
				state = "ready"
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"proof","data":{"state":%q}}`, state)))
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newOnboardingTestClient(srv)
	got, err := c.DiscoverNetworkResource(context.Background(), DiscoveryTestInput{EngineID: "ssh", CredentialID: 7, ConnectorData: json.RawMessage(`{}`), LocationID: 3})
	if err != nil || got.ResourceType != "linux" || taskReads != 2 || proofReads != 2 {
		t.Fatalf("result=%#v taskReads=%d proofReads=%d err=%v", got, taskReads, proofReads, err)
	}
}

func TestDiscoverNetworkResourceRejectsTerminalTaskStatesWithoutLeakingToken(t *testing.T) {
	for _, status := range []string{"failed", "cancelled", "timeout", "no_worker"} {
		t.Run(status, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/resources/test/discovery" {
					_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"started","data":{"task_id":44,"status":"queued","discovery_token":%q}}`, testDiscoveryToken)))
					return
				}
				_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"done","data":{"id":44,"status":%q,"result_success":false,"result_message":"connection failed","error_message":"worker error"}}`, status)))
			}))
			defer srv.Close()

			c := newOnboardingTestClient(srv)
			_, err := c.DiscoverNetworkResource(context.Background(), DiscoveryTestInput{EngineID: "ssh", CredentialID: 7, ConnectorData: json.RawMessage(`{}`), LocationID: 3})
			if err == nil || !strings.Contains(err.Error(), status) {
				t.Fatalf("error = %v, want status %q", err, status)
			}
			if strings.Contains(err.Error(), testDiscoveryToken) {
				t.Fatalf("error leaked discovery token: %v", err)
			}
		})
	}
}

func TestDiscoverNetworkResourceRejectsTerminalProofStates(t *testing.T) {
	for _, state := range []string{"failed", "missing"} {
		t.Run(state, func(t *testing.T) {
			srv := successfulDiscoveryServer(t, state)
			defer srv.Close()
			c := newOnboardingTestClient(srv)
			_, err := c.DiscoverNetworkResource(context.Background(), DiscoveryTestInput{EngineID: "ssh", CredentialID: 7, ConnectorData: json.RawMessage(`{}`), LocationID: 3})
			if err == nil || !strings.Contains(err.Error(), state) {
				t.Fatalf("error = %v, want proof state %q", err, state)
			}
			if strings.Contains(err.Error(), testDiscoveryToken) {
				t.Fatalf("error leaked discovery token: %v", err)
			}
		})
	}
}

func TestDiscoverNetworkResourceHonorsCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/resources/test/discovery" {
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"started","data":{"task_id":45,"status":"queued","discovery_token":%q}}`, testDiscoveryToken)))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"running","data":{"id":45,"status":"running","result_success":null}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test", srv.Client())
	c.pollInterval = time.Second
	c.sleep = func(context.Context, time.Duration) error { return context.Canceled }
	_, err := c.DiscoverNetworkResource(context.Background(), DiscoveryTestInput{EngineID: "ssh", CredentialID: 7, ConnectorData: json.RawMessage(`{}`), LocationID: 3})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestNetworkResourceResponseAcceptsNullExternalID(t *testing.T) {
	var got NetworkResourceResponse
	if err := json.Unmarshal([]byte(`{"id":1,"external_id":null}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.ExternalID != nil {
		t.Fatalf("external ID = %#v, want nil", got.ExternalID)
	}
}

func TestNetworkResourceResponseDecodesEffectiveCoordinates(t *testing.T) {
	var got NetworkResourceResponse
	err := json.Unmarshal([]byte(`{
		"id": 1,
		"effective_coordinates": {
			"latitude": 48.8566,
			"longitude": 2.3522,
			"source_location_id": 7,
			"source_location_name": "Paris"
		}
	}`), &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.EffectiveCoordinates == nil {
		t.Fatal("effective coordinates are nil")
	}
	if got.EffectiveCoordinates.Latitude != 48.8566 || got.EffectiveCoordinates.Longitude != 2.3522 {
		t.Fatalf("coordinates = %#v", got.EffectiveCoordinates)
	}
	if got.EffectiveCoordinates.SourceLocationID == nil || *got.EffectiveCoordinates.SourceLocationID != 7 {
		t.Fatalf("source location id = %#v", got.EffectiveCoordinates.SourceLocationID)
	}
	if got.EffectiveCoordinates.SourceLocationName == nil || *got.EffectiveCoordinates.SourceLocationName != "Paris" {
		t.Fatalf("source location name = %#v", got.EffectiveCoordinates.SourceLocationName)
	}
}

func TestNetworkResourceUpdateInputIsSparse(t *testing.T) {
	name := "new-name"
	body, err := json.Marshal(NetworkResourceUpdateInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"name":"new-name"}` {
		t.Fatalf("update body = %s", body)
	}
}

func TestCreateNetworkResourceUsesProofBackedMisterShell2Contract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/resources" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["resource_type"] != "generic_ssh" || body["discovery_token"] != testDiscoveryToken {
			t.Fatalf("create body = %#v", body)
		}
		if _, exists := body["external_id"]; exists {
			t.Fatalf("create body contains server-managed external_id: %#v", body)
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"created","data":{"id":9,"name":"edge","resource_type":"generic_ssh","connector_id":"ssh","external_id":null,"connector_data":{"host":"192.0.2.10"},"credential_id":7,"location_id":3,"status":"verified","is_enabled":true,"extra_data":null,"created_at":"2026-08-25T10:00:00Z","updated_at":"2026-08-25T10:00:00Z"}}`))
	}))
	defer srv.Close()

	credentialID := int64(7)
	c := NewClient(srv.URL, "test", srv.Client())
	got, err := c.CreateNetworkResource(context.Background(), NetworkResourceCreateInput{
		Name: "edge", ResourceType: "generic_ssh", DiscoveryToken: testDiscoveryToken,
		LocationID: 3, CredentialID: &credentialID, ConnectorData: json.RawMessage(`{"host":"192.0.2.10"}`),
	})
	if err != nil || got.ID != 9 || got.ExternalID != nil {
		t.Fatalf("resource=%#v err=%v", got, err)
	}
}

func newOnboardingTestClient(srv *httptest.Server) *Client {
	c := NewClient(srv.URL, "test", srv.Client())
	c.pollInterval = time.Nanosecond
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func successfulDiscoveryServer(t *testing.T, proofState string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/resources/test/discovery":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"started","data":{"task_id":46,"status":"queued","discovery_token":%q}}`, testDiscoveryToken)))
		case "/api/v1/worker-tasks/46":
			_, _ = w.Write([]byte(`{"success":true,"message":"done","data":{"id":46,"status":"success","result":{"discovery":{"status":"matched","engine_id":"ssh","type_id":"linux"}},"result_success":true}}`))
		case "/api/v1/resources/test/discovery/proof-status":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"proof","data":{"state":%q}}`, proofState)))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
}

func assertOnboardingRequest(t *testing.T, r *http.Request, method, path string) {
	t.Helper()
	if r.Method != method || r.URL.Path != path {
		t.Fatalf("request = %s %s, want %s %s", r.Method, r.URL.Path, method, path)
	}
}
