package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient builds a Client pointed at a test server.
func newTestClient(baseURL string) *Client {
	return NewClient(baseURL, "test-key", &http.Client{})
}

// envelope wraps data in the standard MisterShell response envelope.
func envelope(t *testing.T, data interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	body, err := json.Marshal(apiEnvelope{Success: true, Message: "ok", Data: raw})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return body
}

func TestCreateUser(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/users" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Write(envelope(t, UserResponse{ID: 7, Email: "a@b.com", FirstName: "A", LastName: "B", AuthType: "LOCAL", IsActive: true}))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	u, err := c.CreateUser(context.Background(), UserCreateInput{
		Email: "a@b.com", FirstName: "A", LastName: "B", Password: "supersecret", AuthType: "LOCAL",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID != 7 {
		t.Errorf("id = %d, want 7", u.ID)
	}
	if gotBody["password"] != "supersecret" {
		t.Errorf("password not sent: %v", gotBody["password"])
	}
}

func TestUpdateUserOmitsUnsetFields(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/users/7" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Write(envelope(t, UserResponse{ID: 7, IsActive: false}))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	active := false
	if _, err := c.UpdateUser(context.Background(), 7, UserUpdateInput{IsActive: &active}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	// Only is_active is present; first_name/last_name are omitted (nil pointer + omitempty).
	if _, ok := gotBody["first_name"]; ok {
		t.Errorf("first_name should be omitted, got body %v", gotBody)
	}
	if v, ok := gotBody["is_active"]; !ok || v != false {
		t.Errorf("is_active = %v, want false present", gotBody["is_active"])
	}
}

func TestResetUserPassword(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/users/7/reset-password" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Write(envelope(t, map[string]any{}))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.ResetUserPassword(context.Background(), 7, "newpassword1"); err != nil {
		t.Fatalf("ResetUserPassword: %v", err)
	}
	if gotBody["new_password"] != "newpassword1" {
		t.Errorf("new_password = %v, want newpassword1", gotBody["new_password"])
	}
}

func TestAddAndRemoveUserRole(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/7/roles/3" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		methods = append(methods, r.Method)
		w.Write(envelope(t, map[string]any{}))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.AddUserRole(context.Background(), 7, 3); err != nil {
		t.Fatalf("AddUserRole: %v", err)
	}
	if err := c.RemoveUserRole(context.Background(), 7, 3); err != nil {
		t.Fatalf("RemoveUserRole: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodPut || methods[1] != http.MethodDelete {
		t.Errorf("methods = %v, want [PUT DELETE]", methods)
	}
}
