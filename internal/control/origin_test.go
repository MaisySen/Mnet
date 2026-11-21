package control

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApprovalURLIgnoresCallerOrigin(t *testing.T) {
	st, err := openTestStore(t, t.TempDir()+"/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateNetwork("test"); err != nil {
		t.Fatal(err)
	}
	s, _, err := NewServer(st, nil, "https://control.example.test", "pw")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "http://untrusted.example/api/join", strings.NewReader(`{"network":"test","name":"test-node","pubKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`))
	r.Header.Set("X-Forwarded-Proto", "untrusted")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("join %d %s", rec.Code, rec.Body.String())
	}
	var reply struct {
		ApprovalURL string `json:"approval_url"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(reply.ApprovalURL, "https://control.example.test/join/approve?") {
		t.Fatalf("caller controlled approval origin: %q", reply.ApprovalURL)
	}
}

func TestPublicBaseValidation(t *testing.T) {
	for _, bad := range []string{"", "/relative", "javascript:alert(1)", "https://user:pw@control.test", "https://control.test/path", "https://control.test?query=x", "https://control.test?", "https://control.test/#fragment", "https://:8080"} {
		if _, _, err := NewServer(nil, nil, bad, "pw"); err == nil {
			t.Fatalf("invalid origin accepted: %q", bad)
		}
	}
	st, err := openTestStore(t, t.TempDir()+"/state.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"http://localhost:8080", "https://control.test/", "http://[::1]:80"} {
		s, _, err := NewServer(st, nil, base, "pw")
		if err != nil {
			t.Fatalf("valid origin %q: %v", base, err)
		}
		if got := s.publicURL("/oidc/callback"); got != strings.TrimRight(base, "/")+"/oidc/callback" {
			t.Fatalf("callback URL %s", got)
		}
	}
}
