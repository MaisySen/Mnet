package localapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateTokenPersistsAndRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	token, err := EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EnsureToken(dir)
	if err != nil || again != token {
		t.Fatal("token not stable")
	}
	read, err := ReadToken(dir)
	if err != nil || read != token {
		t.Fatal("token not readable")
	}
	info, err := os.Stat(tokenPath(dir))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("token not private")
	}
	if err = os.WriteFile(tokenPath(dir), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = EnsureToken(dir); err == nil {
		t.Fatal("invalid existing token silently replaced")
	}
	if err = os.Remove(tokenPath(dir)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target")
	if err = os.WriteFile(target, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, tokenPath(dir)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err = EnsureToken(dir); err == nil {
		t.Fatal("token symlink accepted")
	}
}

func TestAPIRequiresCredentialAndRejectsBrowserOrigins(t *testing.T) {
	token, err := EnsureToken(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	called := false
	h := RequireToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) }))
	for _, tc := range []struct {
		name, credential, origin, site string
		want                           int
	}{
		{"missing", "", "", "", 401},
		{"wrong", "wrong", "", "", 401},
		{"valid", token, "", "", 200},
		{"browser with credential", token, "https://untrusted.example", "cross-site", 403},
		{"null origin", token, "null", "", 403},
		{"fetch site", token, "", "same-origin", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest("POST", "http://127.0.0.1:47373/localapi/down", nil)
			r.Header.Set(TokenHeader, tc.credential)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tc.want || called != (tc.want == 200) {
				t.Fatalf("status %d,handler called=%v", rec.Code, called)
			}
			if strings.Contains(rec.Body.String(), token) {
				t.Fatal("capability echoed in error")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientTokenIsLocalOnlyAndDoesNotMutateCaller(t *testing.T) {
	dir := t.TempDir()
	token, err := EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	reached := false
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		reached = true
		if r.Header.Get(TokenHeader) != token {
			t.Error("missing capability")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})
	transport := Transport{StateDir: dir, Base: base}
	for _, target := range []string{"https://untrusted.example/", "http://localhost:47373/", "http://127.0.0.1:9999/"} {
		reached = false
		r, _ := http.NewRequest("GET", target, nil)
		if _, err = transport.RoundTrip(r); err == nil || reached {
			t.Fatal("credential sent to unapproved destination")
		}
	}
	r, _ := http.NewRequest("GET", "http://mnetd/localapi/status", nil)
	resp, err := transport.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if r.Header.Get(TokenHeader) != "" {
		t.Fatal("caller request mutated")
	}
}
