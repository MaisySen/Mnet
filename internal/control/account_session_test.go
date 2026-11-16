package control

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

func accountFixture(t *testing.T) (*Server, *OIDCProvider, string, string) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.CreateNetwork("home")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := store.Join(netmap.JoinRequest{Network: network.ID, Name: "owned", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	p := &OIDCProvider{cfg: OIDCConfig{Issuer: "https://issuer.example"}}
	if err := store.ApproveNodeIdentity(joined.NodeID, "same@example.test", p.cfg.Issuer, "owner"); err != nil {
		t.Fatal(err)
	}
	app, _, err := NewServer(store, relay.NewHub(), "https://control.example", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	app.SetOIDC(p)
	if err := store.SetNetworkMember(network.ID, p.cfg.Issuer, "owner", true); err != nil {
		t.Fatal(err)
	}
	return app, p, network.ID, joined.NodeID
}

func TestAccountSessionScopeRevocationAndSeparation(t *testing.T) {
	app, p, network, node := accountFixture(t)
	token, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "owner", Email: "same@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.accountDevices(token)
	if err != nil || len(got) != 1 || got[0].ID != node {
		t.Fatal("own-device query failed", err)
	}
	got[0].Name = "changed"
	again, _ := app.accountDevices(token)
	if again[0].Name != "owned" {
		t.Fatal("session query aliases private state")
	}
	if _, ok := app.sessions[token]; ok {
		t.Fatal("account became operator session")
	}
	if _, err := app.accountDevices("fixture-device-token"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("device credential became account session")
	}
	if _, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "stranger", Email: "same@example.test"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("matching email granted membership")
	}
	if err := app.store.SetNetworkMember(network, p.cfg.Issuer, "stranger", true); err != nil {
		t.Fatal(err)
	}
	stranger, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "stranger"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := app.accountDevices(stranger)
	if err != nil || len(devices) != 0 {
		t.Fatal("member saw somebody else's devices")
	}
	if err := app.store.SetNetworkMember(network, p.cfg.Issuer, "owner", false); err != nil {
		t.Fatal(err)
	}
	if _, err := app.accountDevices(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("membership revocation did not close active session query")
	}
	app.revokeAccountSession(stranger)
	if _, err := app.accountDevices(stranger); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("logged-out token replay accepted")
	}
}

func TestAccountSessionProviderExpiryCapacity(t *testing.T) {
	app, p, network, _ := accountFixture(t)
	token, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	replacement := &OIDCProvider{cfg: p.cfg}
	app.SetOIDC(replacement)
	if _, err := app.accountDevices(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("provider replacement preserved old session")
	}
	if _, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "owner"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale provider issued session")
	}
	app.SetOIDC(p)
	if _, err := app.accountDevices(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("restoring old provider revived revoked account session")
	}
	token, err = app.newAccountSession(network, p, &OIDCClaims{Subject: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	app.accounts.byToken[token] = accountSession{Expires: time.Now()}
	if _, err := app.accountDevices(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expiry equality accepted")
	}
	if _, ok := app.accounts.byToken[token]; ok {
		t.Fatal("expired session not removed")
	}
	for i := 0; i < maxAccountSessions; i++ {
		app.accounts.byToken[fmt.Sprint(i)] = accountSession{Expires: time.Now().Add(time.Hour)}
	}
	if _, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "owner"}); err == nil {
		t.Fatal("account session capacity exceeded")
	}
	app.accounts.byToken["0"] = accountSession{Expires: time.Now().Add(-time.Second)}
	if _, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "owner"}); err != nil {
		t.Fatal("expired capacity not reclaimed", err)
	}
	for _, claims := range []*OIDCClaims{nil, {Subject: ""}} {
		if _, err := app.newAccountSession(network, p, claims); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("unverified principal accepted")
		}
	}
}

func TestAccountSessionProviderChangeClearsPendingAndLive(t *testing.T) {
	app, p, network, _ := accountFixture(t)
	token, err := app.newAccountSession(network, p, &OIDCClaims{Subject: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	app.accounts.pending = map[string]accountLogin{"pending": {Provider: p, Expires: time.Now().Add(time.Minute)}}
	app.SetOIDC(p)
	if len(app.accounts.pending) != 1 {
		t.Fatal("same provider unnecessarily discarded pending login")
	}
	if _, err := app.accountDevices(token); err != nil {
		t.Fatal("same provider revoked active session")
	}
	app.SetOIDC(nil)
	if len(app.accounts.pending) != 0 || len(app.accounts.byToken) != 0 {
		t.Fatal("provider disable left account credentials")
	}
	app.SetOIDC(p)
	if _, err := app.accountDevices(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("reenabling provider revived old session")
	}
}
