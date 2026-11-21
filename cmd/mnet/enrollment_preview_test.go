package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPreparedEnrollmentIdentityRejectsAmbiguity(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	valid := `{"public_key":"` + key + `"}`
	if b, e := preparedPublicIdentity([]byte(valid)); e != nil || len(b) != 32 {
		t.Fatal("canonical identity refused")
	}
	for _, raw := range []string{`null`, `{}`, `{"Public_key":"` + key + `"}`, `{"public_key":"` + key + `","public_\u006bey":"` + key + `"}`, `{"public_key":"` + key + `","extra":1}`, valid + valid, `{"public_key":null}`, `{"public_key":"` + base64.StdEncoding.EncodeToString(make([]byte, 32)) + `"}`, strings.Repeat(" ", 1025)} {
		if _, e := preparedPublicIdentity([]byte(raw)); e == nil {
			t.Fatal("ambiguous identity accepted")
		}
	}
}
func TestEnrollmentPreviewHasOnlyLocalIdentityAndBrowserEffects(t *testing.T) {
	calls := 0
	key := bytes.Repeat([]byte{9}, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/localapi/identity" || r.URL.RawQuery != "" {
			t.Error("unexpected local effect")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"public_key": base64.StdEncoding.EncodeToString(key)})
	}))
	defer server.Close()
	attempts := map[string]bool{}
	for range 2 {
		var out bytes.Buffer
		e := runEnrollmentPreview(context.Background(), server.Client(), server.URL, "https://multies.example.test", func(_ context.Context, target string) error {
			u, e := url.Parse(target)
			if e != nil || u.Scheme != "https" || u.Host != "multies.example.test" || u.Path != "/mnet/enroll" {
				t.Fatal("wrong browser destination")
			}
			q := u.Query()
			if len(q) != 3 || q.Get("devicePublicKey") != base64.RawURLEncoding.EncodeToString(key) {
				t.Fatal("wrong exact full key")
			}
			id := q.Get("attemptId")
			b, e := base64.RawURLEncoding.Strict().DecodeString(id)
			if e != nil || len(b) != 32 || attempts[id] {
				t.Fatal("attempt was not fresh")
			}
			attempts[id] = true
			return nil
		}, &out)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(out.String(), "No computer will join") || !strings.Contains(out.String(), "has not received or saved enrollment authority") {
			t.Fatal("missing completion boundary")
		}
	}
	if calls != 2 {
		t.Fatal("unexpected local request count")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := runEnrollmentPreview(ctx, server.Client(), server.URL, "https://multies.example.test", func(context.Context, string) error { t.Fatal("canceled browser opened"); return nil }, &bytes.Buffer{}); e == nil || calls != 2 {
		t.Fatal("canceled command had effects")
	}
}
func TestEnrollmentPreviewOriginRequiresExplicitTrustedOrigin(t *testing.T) {
	for _, args := range [][]string{nil, {"--server", "https://host.test/path"}, {"--server", "http://remote.test"}, {"--server", "https://host.test", "--connect"}} {
		if _, e := enrollmentPreviewOrigin(args); e == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
