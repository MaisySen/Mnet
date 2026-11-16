package control_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func accountBrowser(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func accountStart(t *testing.T, c *http.Client, base, network string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"network_id": network})
	resp, err := c.Post(base+"/account/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode != 200 || result["authorization_url"] == "" {
		t.Fatalf("account start failed: %d", resp.StatusCode)
	}
	return result["authorization_url"]
}
func accountCallbackURL(t *testing.T, c *http.Client, authorization string) string {
	t.Helper()
	resp, err := c.Get(authorization)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	loc, err := resp.Location()
	if err != nil {
		t.Fatal(err)
	}
	return loc.String()
}
func accountGet(t *testing.T, c *http.Client, raw string) (int, string) {
	t.Helper()
	resp, err := c.Get(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("account response cached")
	}
	return resp.StatusCode, string(b)
}

func TestAccountHTTPVerifiedLoginMembershipAndLogout(t *testing.T) {
	fake := newFakeOIDC(t, false)
	ts, pw, store := testEnvOIDCStore(t, fake)
	admin := adminLogin(t, ts.URL, pw)
	_, networkBody := post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "account"}, map[string]string{"Cookie": admin.String()})
	var network netmap.Network
	json.Unmarshal(networkBody, &network)
	_, joinBody := post(t, ts.URL+"/api/join", netmap.JoinRequest{Network: network.ID, Name: "mine", PubKey: fakePubKey(0x64)}, nil)
	var joined netmap.JoinResponse
	json.Unmarshal(joinBody, &joined)
	// Existing device approval still grants no account membership.
	if code, _ := followLogin(t, joined.ApprovalURL); code != 200 {
		t.Fatal("fixture approval failed")
	}
	browser := accountBrowser(t)
	callback := accountCallbackURL(t, browser, accountStart(t, browser, ts.URL, network.ID))
	if code, _ := accountGet(t, browser, callback); code != 403 {
		t.Fatal("device approval became account membership")
	}
	if err := store.SetNetworkMember(network.ID, fake.srv.URL, "user-1", true); err != nil {
		t.Fatal(err)
	}
	callback = accountCallbackURL(t, browser, accountStart(t, browser, ts.URL, network.ID))
	// A second browser cannot consume the first browser's flow.
	if code, _ := accountGet(t, accountBrowser(t), callback); code != 400 {
		t.Fatal("callback accepted without originating browser")
	}
	if code, _ := accountGet(t, browser, callback); code != 303 {
		t.Fatal("verified callback failed")
	}
	if code, _ := accountGet(t, browser, callback); code != 400 {
		t.Fatal("callback replay accepted")
	}
	code, body := accountGet(t, browser, ts.URL+"/account/api/devices?subject=stranger&network_id=other")
	if code != 200 || !strings.Contains(body, "mine") {
		t.Fatal("own-device list failed")
	}
	for _, secret := range []string{"node_token", "private_key", "user-1", fake.srv.URL} {
		if strings.Contains(body, secret) {
			t.Fatal("private principal/credential leaked in device list")
		}
	}
	// Query parameters cannot widen scope; current membership remains required.
	if err := store.SetNetworkMember(network.ID, fake.srv.URL, "user-1", false); err != nil {
		t.Fatal(err)
	}
	if code, _ := accountGet(t, browser, ts.URL+"/account/api/devices"); code != 401 {
		t.Fatal("active session bypassed revocation")
	}
	if err := store.SetNetworkMember(network.ID, fake.srv.URL, "user-1", true); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(ts.URL + "/account/api/devices")
	old := append([]*http.Cookie(nil), browser.Jar.Cookies(u)...)
	resp, err := browser.Post(ts.URL+"/account/api/logout", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	replay := accountBrowser(t)
	replay.Jar.SetCookies(u, old)
	if code, _ := accountGet(t, replay, u.String()); code != 401 {
		t.Fatal("logged-out session replay accepted")
	}
}

func TestAccountHTTPMethodsAndOrigin(t *testing.T) {
	fake := newFakeOIDC(t, false)
	ts, _, _ := testEnvOIDCStore(t, fake)
	for _, test := range []struct {
		path, method, origin, media string
		want                        int
	}{
		{"/account/api/login", "GET", "", "", 405},
		{"/account/api/logout", "GET", "", "", 405},
		{"/account/api/devices", "POST", "", "application/json", 405},
		{"/account/oidc/callback", "POST", "", "application/json", 405},
		{"/account/api/login", "POST", "https://other.example", "application/json", 403},
		{"/account/api/logout", "POST", "https://other.example", "application/json", 403},
		{"/account/api/login", "POST", "", "text/plain", 415},
	} {
		req, _ := http.NewRequest(test.method, ts.URL+test.path, strings.NewReader(`{"network_id":"unknown"}`))
		req.Header.Set("Origin", test.origin)
		req.Header.Set("Content-Type", test.media)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != test.want {
			t.Fatalf("%s %s got %d want %d", test.method, test.path, resp.StatusCode, test.want)
		}
	}
}
