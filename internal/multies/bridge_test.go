package multies

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBridgeOriginRestrictions(t *testing.T) {
	for _, s := range []string{"", "http://remote.example", "https://user:pass@example.test", "https://example.test/path", "https://example.test/?x=y", "https://example.test/#fragment", "ftp://localhost", "http://127.1:3000"} {
		if _, err := NewBridgeClient(s); err == nil {
			t.Fatalf("accepted disallowed origin %q", s)
		}
	}
	for _, s := range []string{"https://example.test", "http://127.0.0.1:5179", "http://localhost:5179"} {
		if _, err := NewBridgeClient(s); err != nil {
			t.Fatalf("refused configured origin %q", s)
		}
	}
}

func bridgeFixture(t *testing.T) (*BridgeClient, *httptest.Server, *atomic.Int32) {
	t.Helper()
	var checks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
			t.Error("native carried browser authority")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/mnet/token":
			var p nativeProof
			if json.NewDecoder(r.Body).Decode(&p) != nil || p.Code != thumb("code") || p.WorkspaceID != "workspace" || thumb(p.Verifier) != p.Challenge || p.DeviceKey != thumb("key") {
				http.Error(w, "bad proof", 403)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"credential": thumb("credential"), "tokenType": "Bearer", "expiresIn": 600, "audience": "mnet", "deviceKey": p.DeviceKey, "workspaceId": p.WorkspaceID, "capabilities": []string{"list"}, "inputAuthorized": false})
		case "/api/mnet/check":
			checks.Add(1)
			if r.Header.Get("Authorization") != "Bearer "+thumb("credential") {
				http.Error(w, "wrong token", 403)
				return
			}
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(Evidence{UserID: "person", WorkspaceID: "workspace", DeviceKey: req["deviceKey"], Audience: "mnet", AccountState: "active", WorkspaceState: "active", MembershipState: "active", VerifiedEmail: true, Allowed: []Capability{Capability(req["capability"])}, CheckedAt: time.Now().Add(-time.Millisecond), ExpiresAt: time.Now().Add(25 * time.Second)})
		case "/api/mnet/devices":
			if r.Header.Get("Authorization") != "Bearer "+thumb("credential") {
				http.Error(w, "wrong token", 403)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"workspaceId": "workspace", "devices": []ApprovedDevice{{thumb("id"), thumb("key"), "linux"}}, "ownDevicesOnly": true, "inputAuthorized": false, "truncated": false})
		default:
			http.NotFound(w, r)
		}
	}))
	b, err := NewBridgeClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return b, srv, &checks
}
func finishCallback(t *testing.T, launch string, wrongFirst bool) error {
	t.Helper()
	u, err := url.Parse(launch)
	if err != nil {
		return err
	}
	q := u.Query()
	r, err := url.Parse(q.Get("redirectUri"))
	if err != nil {
		return err
	}
	callback := url.Values{"code": {thumb("code")}, "state": {q.Get("state")}, "workspaceId": {"workspace"}}
	if wrongFirst {
		bad := callback.Encode()
		r.RawQuery = bad + "&state=extra"
		res, err := http.Get(r.String())
		if err != nil {
			return err
		}
		res.Body.Close()
		if res.StatusCode != 400 {
			return fmt.Errorf("duplicate state consumed attempt")
		}
	}
	r.RawQuery = callback.Encode()
	res, err := http.Get(r.String())
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("callback refused")
	}
	return nil
}
func TestBridgeRealLoopbackAndFreshPolicyChecks(t *testing.T) {
	b, srv, checks := bridgeFixture(t)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := b.SignIn(ctx, thumb("key"), "linux", func(s string) error { return finishCallback(t, s, true) })
	if err != nil {
		t.Fatal(err)
	}
	if c.WorkspaceID != "workspace" || c.DeviceKey != thumb("key") {
		t.Fatal("binding lost")
	}
	for _, cap := range []Capability{List, List, List} {
		g, err := b.Authorize(ctx, c, cap)
		if err != nil || g.Capability != cap || g.UserID != "person" {
			t.Fatalf("fresh grant refused: %v", err)
		}
	}
	if checks.Load() != 3 {
		t.Fatal("cached authorization")
	}
	if _, err = b.Authorize(ctx, c, Control); err == nil || checks.Load() != 3 {
		t.Fatal("remote input attempted")
	}
	devices, err := b.Devices(ctx, c)
	if err != nil || len(devices.Devices) != 1 {
		t.Fatal("own devices refused", err)
	}
	for _, s := range []string{fmt.Sprint(c), fmt.Sprintf("%#v", c), func() string { v, _ := json.Marshal(c); return string(v) }()} {
		if strings.Contains(s, c.value) {
			t.Fatal("credential leaked through formatting")
		}
	}
}
func TestBridgeCancelAndBrowserFailureReleaseListener(t *testing.T) {
	b, srv, _ := bridgeFixture(t)
	defer srv.Close()
	for _, cancelBefore := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var callback string
		_, err := b.SignIn(ctx, thumb("key"), "linux", func(s string) error {
			u, _ := url.Parse(s)
			callback = u.Query().Get("redirectUri")
			cancel()
			if cancelBefore {
				return nil
			}
			return ErrDenied
		})
		cancel()
		if err == nil {
			t.Fatal("cancel returned credential")
		}
		cli := &http.Client{Timeout: time.Second}
		if res, e := cli.Get(callback); e == nil {
			res.Body.Close()
			t.Fatal("callback listener survived cancellation")
		}
	}
}
func TestBridgeRefusesRedirectAndOversizedResponse(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	for _, mode := range []string{"redirect", "cookie", "oversize", "input"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "redirect":
					http.Redirect(w, r, target.URL, 307)
				case "cookie":
					w.Header().Set("Set-Cookie", "broad=session")
					fmt.Fprint(w, `{}`)
				case "oversize":
					fmt.Fprint(w, strings.Repeat(" ", 65537))
				case "input":
					fmt.Fprint(w, `{"workspaceId":"workspace","ownDevicesOnly":true,"inputAuthorized":true,"devices":[],"truncated":false}`)
				}
			}))
			defer srv.Close()
			b, _ := NewBridgeClient(srv.URL)
			c := DeviceCredential{issuer: b, value: thumb("credential"), WorkspaceID: "workspace", DeviceKey: thumb("key"), ExpiresAt: time.Now().Add(time.Minute)}
			if _, err := b.Devices(context.Background(), c); err == nil {
				t.Fatal("unsafe response accepted")
			}
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("followed redirect with authority")
	}
}

func TestBridgeCredentialStaysWithItsIssuingClient(t *testing.T) {
	issuer, server, _ := bridgeFixture(t)
	defer server.Close()
	c, err := issuer.SignIn(context.Background(), thumb("key"), "linux", func(s string) error { return finishCallback(t, s, false) })
	if err != nil {
		t.Fatal(err)
	}
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); http.Error(w, "not the issuer", 403) }))
	defer other.Close()
	wrong, err := NewBridgeClient(other.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrong.Authorize(context.Background(), c, List); err == nil {
		t.Fatal("credential switched issuer")
	}
	if _, err = wrong.Devices(context.Background(), c); err == nil {
		t.Fatal("device read switched issuer")
	}
	if received.Load() != 0 {
		t.Fatalf("credential sent to another issuer %d times", received.Load())
	}
}

func TestBridgeCredentialRejectsUnboundAndRecreatedClients(t *testing.T) {
	issuer, server, checks := bridgeFixture(t)
	defer server.Close()
	c, err := issuer.SignIn(context.Background(), thumb("key"), "linux", func(s string) error { return finishCallback(t, s, false) })
	if err != nil {
		t.Fatal(err)
	}
	recreated, err := NewBridgeClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []*BridgeClient{nil, recreated} {
		if _, err = client.Authorize(context.Background(), c, List); err == nil {
			t.Fatal("non-issuing client accepted credential")
		}
		if _, err = client.Devices(context.Background(), c); err == nil {
			t.Fatal("non-issuing client read devices")
		}
	}
	unbound := c
	unbound.issuer = nil
	if _, err = issuer.Authorize(context.Background(), unbound, List); err == nil {
		t.Fatal("unbound credential accepted")
	}
	if _, err = issuer.Devices(context.Background(), unbound); err == nil {
		t.Fatal("unbound credential read devices")
	}
	if checks.Load() != 0 {
		t.Fatal("refused credential reached authorization server")
	}
	if _, err = issuer.Authorize(context.Background(), c, List); err != nil {
		t.Fatal("original bound credential refused", err)
	}
}
