package control_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

func TestOIDCProviderReplacementRetiresPendingAndExchangingApproval(t *testing.T) {
	for _, mode := range []string{"pending-restored", "exchanging-replaced", "exchanging-restored", "exchanging-disabled", "exchanging-same-provider"} {
		t.Run(mode, func(t *testing.T) {
			fake := newFakeOIDC(t, false)
			blocked := mode != "pending-restored"
			if blocked {
				fake.tokenEntered = make(chan struct{}, 1)
				fake.tokenRelease = make(chan struct{})
			}
			var once sync.Once
			release := func() {
				if blocked {
					once.Do(func() { close(fake.tokenRelease) })
				}
			}
			defer release()
			store, err := control.Open(t.TempDir() + "/state.json")
			if err != nil {
				t.Fatal(err)
			}
			ts := httptest.NewUnstartedServer(nil)
			t.Cleanup(ts.Close)
			app, _, err := control.NewServer(store, relay.NewHub(), "http://"+ts.Listener.Addr().String(), "test-admin-pw")
			if err != nil {
				t.Fatal(err)
			}
			provider, err := control.NewOIDCProvider(context.Background(), control.OIDCConfig{Issuer: fake.srv.URL, ClientID: "mnet-test", ClientSecret: "test-secret", AllowedSubjects: []string{"user-1"}})
			if err != nil {
				t.Fatal(err)
			}
			app.SetOIDC(provider)
			ts.Config.Handler = app.Handler()
			ts.Start()
			admin := adminLogin(t, ts.URL, "test-admin-pw")
			_, raw := post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "provider-retirement"}, map[string]string{"Cookie": admin.String()})
			var network netmap.Network
			json.Unmarshal(raw, &network)
			_, raw = post(t, ts.URL+"/api/join", netmap.JoinRequest{Network: network.ID, Name: "pending", PubKey: fakePubKey(0x72)}, nil)
			var joined netmap.JoinResponse
			json.Unmarshal(raw, &joined)
			browser := accountBrowser(t)
			authorization := accountCallbackURL(t, browser, joined.ApprovalURL)
			callback := accountCallbackURL(t, browser, authorization)
			result := make(chan int, 1)
			if blocked {
				go func() {
					response, err := browser.Get(callback)
					if err != nil {
						result <- 0
						return
					}
					response.Body.Close()
					result <- response.StatusCode
				}()
				select {
				case <-fake.tokenEntered:
				case <-time.After(2 * time.Second):
					t.Fatal("provider exchange not entered")
				}
				duplicate, err := browser.Get(callback)
				if err != nil {
					t.Fatal(err)
				}
				duplicate.Body.Close()
				if duplicate.StatusCode != 400 {
					t.Fatal("duplicate callback entered claimed exchange")
				}

			}
			replacement, err := control.NewOIDCProvider(context.Background(), control.OIDCConfig{Issuer: fake.srv.URL, ClientID: "mnet-test", ClientSecret: "test-secret", AllowedSubjects: []string{"user-1"}})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "exchanging-same-provider" {
				app.SetOIDC(provider)
			} else {
				app.SetOIDC(replacement)
			}
			if mode == "pending-restored" || mode == "exchanging-restored" {
				app.SetOIDC(provider)
			}
			if mode == "exchanging-disabled" {
				app.SetOIDC(nil)
			}
			if blocked {
				release()
			} else {
				response, err := browser.Get(callback)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				result <- response.StatusCode
			}
			select {
			case status := <-result:
				if (status == 200) != (mode == "exchanging-same-provider") {
					t.Error("provider retirement/idempotence returned wrong approval result", status)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("retired callback did not finish")
			}
			if identity, exists := store.NodeApprovalIdentity(joined.NodeID); exists != (mode == "exchanging-same-provider") {
				t.Error("provider retirement/idempotence persisted wrong identity", identity)
			}
			for _, node := range store.Snapshot().Nodes {
				if node.ID == joined.NodeID && node.Approved && mode != "exchanging-same-provider" {
					t.Error("retired provider approved waiting node")
				}
			}
			replay, err := browser.Get(callback)
			if err != nil {
				t.Fatal(err)
			}
			replay.Body.Close()
			if replay.StatusCode == 200 {
				t.Fatal("callback replay approved again")
			}

			app.SetOIDC(replacement)
			if code, _ := followLogin(t, joined.ApprovalURL); code != 200 {
				t.Fatal("new current-provider approval failed after retirement")
			}
			persisted, err := control.Open(store.Path())
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := persisted.NodeApprovalIdentity(joined.NodeID); !exists {
				t.Fatal("fresh verified approval was not persisted")
			}
		})
	}
}
