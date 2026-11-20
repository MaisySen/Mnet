package multies

// Desktop handoff for the separately gated Multies bridge. It is not configured
// by default and does not enroll a VPN peer or grant screen/input/file access.
import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type BridgeClient struct {
	origin string
	http   *http.Client
}

// NewBridgeClient accepts an explicit trusted origin only. Plain HTTP is allowed
// solely for a local development bridge, never a remote server. No cookie jar,
// caller transport, redirects or browser-session credentials are inherited.
func NewBridgeClient(origin string) (*BridgeClient, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return nil, ErrDenied
	}
	local := u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, ErrDenied
	}
	if u.Port() != "" {
		n, e := strconv.Atoi(u.Port())
		if e != nil || n < 1 || n > 65535 {
			return nil, ErrDenied
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	return &BridgeClient{origin: strings.TrimSuffix(u.String(), "/"), http: &http.Client{Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type DeviceCredential struct {
	value                  string
	WorkspaceID, DeviceKey string
	ExpiresAt              time.Time
}

func (DeviceCredential) String() string   { return "[redacted Mnet credential]" }
func (DeviceCredential) GoString() string { return "[redacted Mnet credential]" }
func (DeviceCredential) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted Mnet credential]"`), nil
}

var _ fmt.Stringer = DeviceCredential{}

func opaque() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", ErrDenied
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func thumb(s string) string {
	h := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

type launchRequest struct {
	DeviceKey   string `json:"deviceKey"`
	Challenge   string `json:"challenge"`
	State       string `json:"state"`
	Platform    string `json:"platform"`
	RedirectURI string `json:"redirectUri"`
}
type nativeProof struct {
	launchRequest
	WorkspaceID string `json:"workspaceId"`
	Code        string `json:"code"`
	Verifier    string `json:"verifier"`
}
type handoffResult struct {
	code, workspace string
	declined        bool
}

// SignIn binds a fresh native proof to an already-bound loopback listener before
// opening the browser. The opener must show the exact device thumbprint in the
// native UI before opening this URL. Only a matching one-use callback can finish.
func (b *BridgeClient) SignIn(ctx context.Context, deviceKey, platform string, open func(string) error) (DeviceCredential, error) {
	deny := func() (DeviceCredential, error) { return DeviceCredential{}, ErrDenied }
	if b == nil || b.http == nil || ctx == nil || ctx.Err() != nil || open == nil || !validDeviceKey(deviceKey) || (platform != "linux" && platform != "windows" && platform != "macos") {
		return deny()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	verifier, err := opaque()
	if err != nil {
		return deny()
	}
	state, err := opaque()
	if err != nil {
		return deny()
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return deny()
	}
	defer listener.Close()
	host := listener.Addr().String()
	request := launchRequest{deviceKey, thumb(verifier), state, platform, "http://" + host + "/mnet/callback"}
	arrived := make(chan handoffResult, 1)
	var once sync.Once
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		fail := func() { http.Error(w, "Mnet sign in callback refused.", http.StatusBadRequest) }
		if r.Method != http.MethodGet || r.Host != host || r.URL.Path != "/mnet/callback" || r.URL.RawPath != "" || r.Header.Get("Origin") != "" || len(r.URL.RawQuery) > 1024 {
			fail()
			return
		}
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(q) < 2 || len(q) > 3 {
			fail()
			return
		}
		for k, v := range q {
			if len(v) != 1 || (k != "state" && k != "workspaceId" && k != "code" && k != "error") {
				fail()
				return
			}
		}
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			fail()
			return
		}
		denied := q.Get("error") == "access_denied"
		if (denied && (q.Has("code") || (q.Has("workspaceId") && !idPattern.MatchString(q.Get("workspaceId"))))) || (!denied && (!validDeviceKey(q.Get("code")) || !idPattern.MatchString(q.Get("workspaceId")) || q.Has("error"))) {
			fail()
			return
		}
		won := false
		once.Do(func() { won = true; arrived <- handoffResult{q.Get("code"), q.Get("workspaceId"), denied} })
		if !won {
			fail()
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Mnet received the sign in response. You can return to Mnet.\n")
	})
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan struct{})
	go func() { defer close(done); _ = srv.Serve(listener) }()
	defer func() {
		stop, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(stop)
		_ = srv.Close()
		<-done
	}()
	q := url.Values{"deviceKey": {request.DeviceKey}, "challenge": {request.Challenge}, "state": {request.State}, "platform": {request.Platform}, "redirectUri": {request.RedirectURI}}
	if open(b.origin+"/mnet/authorize?"+q.Encode()) != nil {
		return deny()
	}
	var result handoffResult
	select {
	case <-ctx.Done():
		return deny()
	case result = <-arrived:
	}
	if result.declined || ctx.Err() != nil {
		return deny()
	}
	started := time.Now()
	var response struct {
		Credential      string       `json:"credential"`
		TokenType       string       `json:"tokenType"`
		ExpiresIn       int          `json:"expiresIn"`
		Audience        string       `json:"audience"`
		DeviceKey       string       `json:"deviceKey"`
		WorkspaceID     string       `json:"workspaceId"`
		Capabilities    []Capability `json:"capabilities"`
		InputAuthorized *bool        `json:"inputAuthorized"`
	}
	if b.call(ctx, http.MethodPost, "/api/mnet/token", "", nativeProof{request, result.workspace, result.code, verifier}, &response) != nil {
		return deny()
	}
	if ctx.Err() != nil || !validDeviceKey(response.Credential) || response.TokenType != "Bearer" || response.ExpiresIn < 1 || response.ExpiresIn > 600 || response.Audience != "mnet" || response.DeviceKey != deviceKey || response.WorkspaceID != result.workspace || response.InputAuthorized == nil || *response.InputAuthorized || len(response.Capabilities) != 1 || response.Capabilities[0] != List {
		return deny()
	}
	return DeviceCredential{response.Credential, response.WorkspaceID, deviceKey, started.Add(time.Duration(response.ExpiresIn) * time.Second)}, nil
}

func (b *BridgeClient) call(ctx context.Context, method, path, token string, body any, out any) error {
	if b == nil || b.http == nil || ctx == nil || ctx.Err() != nil {
		return ErrDenied
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil || len(raw) > 8192 {
			return ErrDenied
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.origin+path, reader)
	if err != nil {
		return ErrDenied
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		if !validDeviceKey(token) {
			return ErrDenied
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := b.http.Do(req)
	if err != nil {
		return ErrDenied
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Set-Cookie") != "" {
		return ErrDenied
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return ErrDenied
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(out) != nil {
		return ErrDenied
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return ErrDenied
	}
	if ctx.Err() != nil {
		return ErrDenied
	}
	return nil
}

type ApprovedDevice struct {
	ID        string `json:"id"`
	DeviceKey string `json:"deviceKey"`
	Platform  string `json:"platform"`
}
type ApprovedDeviceList struct {
	WorkspaceID     string           `json:"workspaceId"`
	Devices         []ApprovedDevice `json:"devices"`
	OwnDevicesOnly  bool             `json:"ownDevicesOnly"`
	InputAuthorized *bool            `json:"inputAuthorized"`
	Truncated       bool             `json:"truncated"`
}

func (b *BridgeClient) Devices(ctx context.Context, c DeviceCredential) (ApprovedDeviceList, error) {
	var out ApprovedDeviceList
	if !c.ExpiresAt.After(time.Now()) || !idPattern.MatchString(c.WorkspaceID) || !validDeviceKey(c.DeviceKey) || b.call(ctx, http.MethodGet, "/api/mnet/devices", c.value, nil, &out) != nil {
		return ApprovedDeviceList{}, ErrDenied
	}
	if out.WorkspaceID != c.WorkspaceID || !out.OwnDevicesOnly || out.InputAuthorized == nil || *out.InputAuthorized || len(out.Devices) > 100 {
		return ApprovedDeviceList{}, ErrDenied
	}
	seen := map[string]bool{}
	for _, d := range out.Devices {
		if !validDeviceKey(d.ID) || !validDeviceKey(d.DeviceKey) || seen[d.ID] || (d.Platform != "linux" && d.Platform != "windows" && d.Platform != "macos") {
			return ApprovedDeviceList{}, ErrDenied
		}
		seen[d.ID] = true
	}
	if ctx.Err() != nil {
		return ApprovedDeviceList{}, ErrDenied
	}
	return out, nil
}

// Verify is a live server check for one exact policy operation. Nothing is
// cached and no broader input, view, transfer or record request is attempted.
func (b *BridgeClient) Verify(ctx context.Context, request Request) (Evidence, error) {
	if request.Capability != List {
		return Evidence{}, ErrDenied
	}
	body := map[string]string{"workspaceId": request.WorkspaceID, "capability": string(request.Capability)}
	if request.DeviceKey != "" {
		body["deviceKey"] = request.DeviceKey
	}
	var out Evidence
	if b.call(ctx, http.MethodPost, "/api/mnet/check", request.Credential, body, &out) != nil {
		return Evidence{}, ErrDenied
	}
	return out, nil
}

// Authorize keeps the bearer value inside this transport; callers get only the
// exact short-lived policy grant. This does not execute network enrollment.
func (b *BridgeClient) Authorize(ctx context.Context, c DeviceCredential, capability Capability) (Grant, error) {
	if !c.ExpiresAt.After(time.Now()) {
		return Grant{}, ErrDenied
	}
	key := c.DeviceKey
	if capability == List {
		key = ""
	}
	return (Policy{Verifier: b}).Authorize(ctx, Request{Credential: c.value, WorkspaceID: c.WorkspaceID, DeviceKey: key, Capability: capability})
}
