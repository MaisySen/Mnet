package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMultiesSignInOriginIsExplicitAndOriginOnly(t *testing.T) {
	for _, args := range [][]string{nil, {"--server"}, {"--server", "http://remote.example"}, {"--server", "https://example.test/path"}, {"--server", "https://user:pass@example.test"}, {"--server", "https://example.test", "--persist"}} {
		if _, err := multiesSignInOrigin(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if got, err := multiesSignInOrigin([]string{"--server", "http://127.0.0.1:5179"}); err != nil || got != "http://127.0.0.1:5179" {
		t.Fatal("explicit local bridge refused")
	}
}
func TestMultiesSignInRequiresCanonicalPreparedIdentityBeforeBrowser(t *testing.T) {
	for _, key := range []string{"", "not-key", base64.StdEncoding.EncodeToString(make([]byte, 32)) + "\n"} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != "POST" || r.URL.Path != "/localapi/identity" {
				t.Error("unexpected local operation")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "down", "public_key": key})
		}))
		opened := false
		_, err := runMultiesSignIn(context.Background(), server.Client(), server.URL, "http://localhost:5179", func(context.Context, string) error { opened = true; return nil })
		server.Close()
		if err == nil || opened || calls != 1 {
			t.Fatal("accepted missing/invalid device identity")
		}
	}
}
