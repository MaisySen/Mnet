package control

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOIDCStateBoundsExpiryAndProviderScope(t *testing.T) {
	st := scaleStore(t, 1)
	app, _, err := NewServer(st, nil, "http://localhost", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	provider := &OIDCProvider{authURL: "https://issuer.invalid/auth"}
	app.SetOIDC(provider)
	startURL := "http://localhost/oidc/start?node_id=node-000000&token=synthetic-benchmark-token"
	for i := 0; i < maxPendingOIDCStates; i++ {
		app.oidcStates[fmt.Sprint(i)] = oidcState{NodeID: "node-000000", Exp: time.Now().Add(time.Hour), Provider: provider}
	}
	request := func(method, u string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, httptest.NewRequest(method, u, nil))
		return rec
	}
	for _, method := range []string{"POST", "HEAD", "DELETE"} {
		if rec := request(method, startURL); rec.Code != 405 {
			t.Fatalf("start method: %d", rec.Code)
		}
		if rec := request(method, "http://localhost/oidc/callback?state=0"); rec.Code != 405 {
			t.Fatalf("callback method: %d", rec.Code)
		}
	}
	if len(app.oidcStates) != maxPendingOIDCStates {
		t.Fatal("wrong method consumed or allocated state")
	}
	rec := request("GET", startURL)
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" || len(app.oidcStates) != maxPendingOIDCStates {
		t.Fatal("state table did not fail closed at bound")
	}
	// Expired entries are reclaimed before capacity is checked.
	app.oidcStates["0"] = oidcState{Exp: time.Now(), Provider: provider}
	rec = request("GET", startURL)
	if rec.Code != 302 || len(app.oidcStates) != maxPendingOIDCStates {
		t.Fatalf("expired slot not reclaimed: %d", rec.Code)
	}
	if _, exists := app.oidcStates["0"]; exists {
		t.Fatal("expired state survived")
	}
	// The newly admitted state binds the exact provider instance.
	fresh := 0
	for key, state := range app.oidcStates {
		if len(key) < 8 {
			continue
		}
		fresh++
		if state.Provider != provider {
			t.Fatal("state lost provider binding")
		}
	}
	if fresh != 1 {
		t.Fatalf("expected one fresh state: %d", fresh)
	}
	for _, tc := range []struct {
		name  string
		state oidcState
	}{
		{"expired", oidcState{NodeID: "node-000000", Exp: time.Now(), Provider: provider}},
		{"provider replaced", oidcState{NodeID: "node-000000", Exp: time.Now().Add(time.Hour), Provider: &OIDCProvider{}}},
		{"legacy unbound", oidcState{NodeID: "node-000000", Exp: time.Now().Add(time.Hour)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app.oidcStates["rejected"] = tc.state
			if rec := request("GET", "http://localhost/oidc/callback?state=rejected&code=unused"); rec.Code != 400 {
				t.Fatalf("callback: %d", rec.Code)
			}
			if _, exists := app.oidcStates["rejected"]; exists {
				t.Fatal("rejected state reusable")
			}
		})
	}
	// Provider error also consumes an otherwise valid state once.
	app.oidcStates["denied"] = oidcState{NodeID: "node-000000", Exp: time.Now().Add(time.Hour), Provider: provider}
	if rec := request("GET", "http://localhost/oidc/callback?state=denied&error=access_denied"); rec.Code != 403 {
		t.Fatalf("provider denial: %d", rec.Code)
	}
	if rec := request("GET", "http://localhost/oidc/callback?state=denied&error=access_denied"); rec.Code != 400 {
		t.Fatal("state replay accepted")
	}
}
