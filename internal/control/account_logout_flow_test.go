package control_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestAccountLogoutRetiresPendingAndExchangingBrowserLogin(t *testing.T) {
	for _, mode := range []string{"pending", "exchanging"} {
		t.Run(mode, func(t *testing.T) {
			fake := newFakeOIDC(t, false)
			if mode == "exchanging" {
				fake.tokenEntered = make(chan struct{}, 1)
				fake.tokenRelease = make(chan struct{})
			}
			var releaseOnce sync.Once
			release := func() {
				if fake.tokenRelease != nil {
					releaseOnce.Do(func() { close(fake.tokenRelease) })
				}
			}
			defer release()
			ts, pw, store := testEnvOIDCStore(t, fake)
			admin := adminLogin(t, ts.URL, pw)
			_, body := post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "logout-flow"}, map[string]string{"Cookie": admin.String()})
			var network netmap.Network
			json.Unmarshal(body, &network)
			if err := store.SetNetworkMember(network.ID, fake.srv.URL, "user-1", true); err != nil {
				t.Fatal(err)
			}
			browser := accountBrowser(t)
			callback := accountCallbackURL(t, browser, accountStart(t, browser, ts.URL, network.ID))
			u, _ := url.Parse(ts.URL + "/account/")
			proof := browser.Jar.Cookies(u)
			stale := accountBrowser(t)
			stale.Jar.SetCookies(u, proof)
			callbackResult := make(chan int, 1)
			if mode == "exchanging" {
				go func() {
					r, err := stale.Get(callback)
					if err != nil {
						callbackResult <- 0
						return
					}
					r.Body.Close()
					callbackResult <- r.StatusCode
				}()
				select {
				case <-fake.tokenEntered:
				case <-time.After(2 * time.Second):
					t.Fatal("provider exchange not entered")
				}
			}
			response, err := browser.Post(ts.URL+"/account/api/logout", "application/json", strings.NewReader(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatal("logout failed")
			}
			if mode == "exchanging" {
				release()
			} else {
				r, err := stale.Get(callback)
				if err != nil {
					t.Fatal(err)
				}
				r.Body.Close()
				callbackResult <- r.StatusCode
			}
			select {
			case status := <-callbackResult:
				if status == http.StatusSeeOther {
					t.Error("pre-logout callback restored account session")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("callback retained after logout")
			}
			if code, _ := accountGet(t, stale, ts.URL+"/account/api/devices"); code != 401 {
				t.Fatal("late callback session usable after logout")
			}
			for _, cookie := range browser.Jar.Cookies(u) {
				if cookie.Name == "mnet_account_login" {
					t.Error("logout retained login proof cookie")
				}
			}
			// A deliberate fresh sign-in after logout remains supported.
			next := accountCallbackURL(t, browser, accountStart(t, browser, ts.URL, network.ID))
			if code, _ := accountGet(t, browser, next); code != 303 {
				t.Fatal("fresh login after logout refused")
			}
		})
	}
}

func TestAccountNewLoginRetiresSupersededBrowserExchange(t *testing.T) {
	fake := newFakeOIDC(t, false)
	fake.tokenEntered = make(chan struct{}, 1)
	fake.tokenRelease = make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(fake.tokenRelease) }) }
	defer release()
	ts, pw, store := testEnvOIDCStore(t, fake)
	admin := adminLogin(t, ts.URL, pw)
	_, body := post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "superseded-flow"}, map[string]string{"Cookie": admin.String()})
	var network netmap.Network
	json.Unmarshal(body, &network)
	if err := store.SetNetworkMember(network.ID, fake.srv.URL, "user-1", true); err != nil {
		t.Fatal(err)
	}
	browser := accountBrowser(t)
	first := accountCallbackURL(t, browser, accountStart(t, browser, ts.URL, network.ID))
	u, _ := url.Parse(ts.URL + "/account/")
	stale := accountBrowser(t)
	stale.Jar.SetCookies(u, browser.Jar.Cookies(u))
	result := make(chan int, 1)
	go func() {
		r, err := stale.Get(first)
		if err != nil {
			result <- 0
			return
		}
		r.Body.Close()
		result <- r.StatusCode
	}()
	select {
	case <-fake.tokenEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("exchange not entered")
	}
	second := accountCallbackURL(t, browser, accountStart(t, browser, ts.URL, network.ID))
	response, err := browser.Post(ts.URL+"/account/api/logout", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	release()
	var status int
	select {
	case status = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("superseded callback did not finish")
	}
	if status == 303 {
		t.Error("superseded callback restored session after newer-flow logout")
	}
	if code, _ := accountGet(t, stale, ts.URL+"/account/api/devices"); code != 401 {
		t.Fatal("superseded browser session usable")
	}
	if code, _ := accountGet(t, browser, second); code == 303 {
		t.Fatal("logged-out newer flow accepted")
	}
}

func TestAccountLogoutDoesNotRetireOtherBrowser(t *testing.T) {
	fake := newFakeOIDC(t, false)
	ts, pw, store := testEnvOIDCStore(t, fake)
	admin := adminLogin(t, ts.URL, pw)
	_, body := post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "isolated-logout"}, map[string]string{"Cookie": admin.String()})
	var network netmap.Network
	json.Unmarshal(body, &network)
	if err := store.SetNetworkMember(network.ID, fake.srv.URL, "user-1", true); err != nil {
		t.Fatal(err)
	}
	active := accountBrowser(t)
	pending := accountBrowser(t)
	leaving := accountBrowser(t)
	activeCallback := accountCallbackURL(t, active, accountStart(t, active, ts.URL, network.ID))
	if code, _ := accountGet(t, active, activeCallback); code != 303 {
		t.Fatal("other browser initial login failed")
	}
	pendingCallback := accountCallbackURL(t, pending, accountStart(t, pending, ts.URL, network.ID))
	leavingCallback := accountCallbackURL(t, leaving, accountStart(t, leaving, ts.URL, network.ID))
	response, err := leaving.Post(ts.URL+"/account/api/logout", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if code, _ := accountGet(t, active, ts.URL+"/account/api/devices"); code != 200 {
		t.Fatal("logout revoked unrelated browser session")
	}
	if code, _ := accountGet(t, pending, pendingCallback); code != 303 {
		t.Fatal("logout retired unrelated pending browser")
	}
	if code, _ := accountGet(t, leaving, leavingCallback); code == 303 {
		t.Fatal("signed-out browser login restored")
	}
}
