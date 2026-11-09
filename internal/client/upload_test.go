package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigurationUploadReplaysBinaryOnRateLimit(t *testing.T) {
	data := []byte{0, 255, 1, 10}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/v1/files/catalog/upload" || r.URL.Query().Get("name") != "demo.bin" {
			t.Errorf("wrong upload target: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Error("missing upload authentication/content type")
		}
		got, _ := io.ReadAll(r.Body)
		if !bytes.Equal(got, data) {
			t.Errorf("incorrect payload: %v", got)
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		io.WriteString(w, `{"success":true,"data":{"id":7}}`)
	}))
	defer server.Close()
	c := NewClient(server.URL, "test-token", server.Client())
	got, err := c.ConfigurationUpload(context.Background(), "/api/v1/files/catalog/upload?name=demo.bin", bytes.NewReader(data))
	if err != nil || string(got) != `{"id":7}` || calls != 2 {
		t.Fatalf("got %s, err %v, calls %d", got, err, calls)
	}
}

type borrowedUpload struct {
	*bytes.Reader
	closed bool
}

func (b *borrowedUpload) Close() error { b.closed = true; return nil }
func (b *borrowedUpload) Seek(offset int64, whence int) (int64, error) {
	if b.closed {
		return 0, fmt.Errorf("closed upload")
	}
	return b.Reader.Seek(offset, whence)
}
func TestUploadRetainsCallerOwnedReaderAcrossRetries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.Copy(io.Discard, r.Body)
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		io.WriteString(w, `{"success":true,"data":{"id":1}}`)
	}))
	defer server.Close()
	reader := &borrowedUpload{Reader: bytes.NewReader([]byte("payload"))}
	_, err := NewClient(server.URL, "token", server.Client()).ConfigurationUpload(context.Background(), "/upload", reader)
	if err != nil || reader.closed || calls != 2 {
		t.Fatalf("upload err=%v closed=%v attempts=%d", err, reader.closed, calls)
	}
}
func TestMalformedResponseDoesNotDiscloseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":true,"data":{"content":"private-note-secret`)
	}))
	defer server.Close()
	_, err := NewClient(server.URL, "token", server.Client()).ConfigurationRequest(context.Background(), "GET", "/api/v1/notes/location/1", nil)
	if err == nil || strings.Contains(err.Error(), "private-note-secret") {
		t.Fatalf("expected sanitized parse failure, got %v", err)
	}
}
