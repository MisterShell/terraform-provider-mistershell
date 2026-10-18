package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLongUnicodeLookupSearchBounded(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		search := r.URL.Query().Get("search")
		if utf8.RuneCountInString(search) > 100 {
			t.Errorf("oversized API search %d", utf8.RuneCountInString(search))
			w.WriteHeader(422)
			return
		}
		if search != strings.Repeat("é", 100) {
			t.Errorf("wrong search prefix %q", search)
		}
		w.Write([]byte(`{"success":true,"data":[],"meta":{"pages":1}}`))
	}))
	defer s.Close()
	_, err := NewClient(s.URL, "test", s.Client()).ListExternalVaults(context.Background(), ExternalVaultListFilter{Search: SearchPrefix(strings.Repeat("é", 110))})
	if err != nil {
		t.Fatal(err)
	}
}
