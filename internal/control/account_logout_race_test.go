package control

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccountCallbackSessionIssuanceCannotOutrunBrowserLogout(t *testing.T) {
	app, p, network, _ := accountFixture(t)
	hash := sha256.Sum256([]byte("originating-browser-proof"))
	// Both lock orders are valid: logout either retires the exchange first or
	// revokes its newly issued token even before that token reaches the browser.
	for i := 0; i < 1000; i++ {
		state := fmt.Sprintf("callback-%d", i)
		login := accountLogin{NetworkID: network, Provider: p, BrowserHash: hash, Expires: time.Now().Add(time.Minute), claimed: true}
		app.accounts.mu.Lock()
		app.accounts.pending = map[string]accountLogin{state: login}
		app.accounts.mu.Unlock()
		start := make(chan struct{})
		var group sync.WaitGroup
		group.Add(2)
		var token string
		go func() {
			defer group.Done()
			<-start
			token, _ = app.newAccountSessionFromLogin(network, p, &OIDCClaims{Subject: "owner"}, state, &login)
		}()
		go func() {
			defer group.Done()
			<-start
			request := httptest.NewRequest("POST", "https://control.example/account/api/logout", strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			request.AddCookie(&http.Cookie{Name: "mnet_account_login", Value: "originating-browser-proof"})
			app.handleAccountLogout(httptest.NewRecorder(), request)
		}()
		close(start)
		group.Wait()
		if token != "" {
			if _, err := app.accountDevices(token); err == nil {
				t.Fatal("session minted around logout remained usable")
			}
		}
		app.accounts.mu.Lock()
		remaining := len(app.accounts.pending) + len(app.accounts.byToken)
		app.accounts.mu.Unlock()
		if remaining != 0 {
			t.Fatal("logout retained callback ownership or issued token")
		}
	}
}

func TestAccountCallbackSessionRequiresExactLiveClaimedAttempt(t *testing.T) {
	app, p, network, _ := accountFixture(t)
	original := accountLogin{NetworkID: network, Provider: p, BrowserHash: sha256.Sum256([]byte("proof")), Expires: time.Now().Add(time.Minute), claimed: true}
	for _, mode := range []string{"retired", "unclaimed", "expired", "changed-browser", "changed-network", "changed-provider"} {
		t.Run(mode, func(t *testing.T) {
			login := original
			app.accounts.pending = map[string]accountLogin{"state": original}
			switch mode {
			case "retired":
				delete(app.accounts.pending, "state")
			case "unclaimed":
				login.claimed = false
				app.accounts.pending["state"] = login
			case "expired":
				login.Expires = time.Now()
				app.accounts.pending["state"] = login
			case "changed-browser":
				login.BrowserHash = sha256.Sum256([]byte("other"))
			case "changed-network":
				login.NetworkID = "other"
			case "changed-provider":
				login.Provider = &OIDCProvider{}
			}
			if token, err := app.newAccountSessionFromLogin(network, p, &OIDCClaims{Subject: "owner"}, "state", &login); err == nil || token != "" {
				t.Fatal("changed/retired callback issued account session")
			}
		})
	}
}
