// Package enrollmentsource implements the explicit-origin native exchange
// transport only. It has no runtime caller, journal access, authority permit,
// service credential, browser, redirect or automatic retry/reconsent path.
package enrollmentsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"regexp"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/enrollmentidentity"
)

var ErrUnavailable = errors.New("enrollment exchange unavailable; remote outcome may be unknown")
var ErrInvalid = errors.New("invalid enrollment exchange request")

// Limits are explicit engineering transport bounds, not an authority lifetime,
// observation freshness, lease policy or production product default.
type Limits struct {
	Timeout        time.Duration
	MaxConnections int
}
type Native struct {
	origin, authority string
	http              *http.Client
	transport         *http.Transport
}

func (Native) String() string   { return "[explicit-origin native enrollment exchange client]" }
func (Native) GoString() string { return "[explicit-origin native enrollment exchange client]" }
func NewNative(origin, principalAuthority string, limits Limits) (*Native, error) {
	if !enrollmentidentity.ValidOrigin(origin) || !enrollmentidentity.ValidOrigin(principalAuthority) || limits.Timeout <= 0 || limits.Timeout > time.Minute || limits.MaxConnections < 1 || limits.MaxConnections > 16 {
		return nil, ErrInvalid
	}
	// Construct independently: no DefaultTransport/DefaultClient/ProxyFromEnvironment,
	// caller RoundTripper, cookie jar, ambient browser authentication or TLS bypass.
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: limits.Timeout, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: limits.Timeout, ResponseHeaderTimeout: limits.Timeout, MaxResponseHeaderBytes: 16 * 1024, MaxConnsPerHost: limits.MaxConnections, MaxIdleConnsPerHost: limits.MaxConnections, IdleConnTimeout: 30 * time.Second, DisableCompression: true}
	h := &http.Client{Transport: tr, Timeout: limits.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Native{origin: origin, authority: principalAuthority, http: h, transport: tr}, nil
}

// CloseIdleConnections releases pooled connections, not pending journal state.
func (n *Native) CloseIdleConnections() {
	if n != nil && n.transport != nil {
		n.transport.CloseIdleConnections()
	}
}

var verifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

func canonicalCode(s string) bool {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != s {
		return false
	}
	for _, v := range b {
		if v != 0 {
			return true
		}
	}
	return false
}

// Exchange performs ONE explicit one-shot request. Caller MUST first durably
// record exchange-may-have-started and independently own saved key/secret/PKCE/
// listener/config. This function cannot prove those preconditions. Code/verifier
// are private native inputs; request and response bodies must never be logged.
// Any network/protocol/status failure is outcome-unknown, not permission to retry.
// now is caller DATA, not a trusted clock. Returned exported DATA is forgeable and
// must not be converted to an authority permit or native credential installation.
func (n *Native) Exchange(ctx context.Context, saved contract.SourceLaunch, savedSourceOrigin, savedAuthority, code, verifier string, savedAllowed, currentAllowed []contract.SourceMapping, now time.Time) (contract.SourceExchange, error) {
	if n == nil || n.http == nil || n.origin != savedSourceOrigin || n.authority != savedAuthority || ctx == nil || ctx.Err() != nil || !canonicalCode(code) || !verifierPattern.MatchString(verifier) || now.IsZero() {
		return contract.SourceExchange{}, ErrInvalid
	}
	sum := sha256.Sum256([]byte(verifier))
	if saved.Challenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
		return contract.SourceExchange{}, ErrInvalid
	}
	if len(savedAllowed) == 0 || len(currentAllowed) == 0 || len(savedAllowed) > 20 || len(currentAllowed) > 20 {
		return contract.SourceExchange{}, ErrInvalid
	}
	// Reuse DATA-only descriptor shape validation before disclosing any private
	// code/verifier. This does not invent a PKCE result or authenticate config.
	for _, list := range [][]contract.SourceMapping{savedAllowed, currentAllowed} {
		for _, m := range list {
			if _, err := contract.SourceDescriptorBytes(contract.SourceExchange{ID: saved.AttemptID, Launch: saved, Mapping: m}, "native-request-shape", saved.AttemptID); err != nil {
				return contract.SourceExchange{}, ErrInvalid
			}
		}
	}
	intersection := false
	for _, original := range savedAllowed {
		for _, current := range currentAllowed {
			if original == current {
				intersection = true
			}
		}
	}
	if !intersection {
		return contract.SourceExchange{}, ErrInvalid
	}
	payload := struct {
		contract.SourceLaunch
		Code     string `json:"code"`
		Verifier string `json:"verifier"`
	}{saved, code, verifier}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > 16384 {
		return contract.SourceExchange{}, ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.origin+"/api/mnet-enrollment-v1/exchange-v2", bytes.NewReader(raw))
	if err != nil {
		return contract.SourceExchange{}, ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Pragma", "no-cache")
	started := time.Now()
	response, err := n.http.Do(req)
	if err != nil {
		return contract.SourceExchange{}, ErrUnavailable
	}
	defer response.Body.Close()
	// Never surface a server error body, Location, raw url.Error or response data.
	// No body drain/retry on errors: data may contain private tokens or instructions.
	if response.StatusCode != http.StatusOK || response.ContentLength > 65536 || response.Header.Get("Content-Encoding") != "" {
		return contract.SourceExchange{}, ErrUnavailable
	}
	if len(response.Header.Values("Content-Type")) != 1 || len(response.Header.Values("Cache-Control")) != 1 {
		return contract.SourceExchange{}, ErrUnavailable
	}
	typ, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" || len(params) > 1 || (len(params) == 1 && params["charset"] != "utf-8") {
		return contract.SourceExchange{}, ErrUnavailable
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		return contract.SourceExchange{}, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 {
		return contract.SourceExchange{}, ErrUnavailable
	}
	data, err := contract.DecodeSourceExchange(body, saved, n.authority, savedAllowed, currentAllowed, now.Add(time.Since(started)))
	if err != nil {
		return contract.SourceExchange{}, ErrUnavailable
	}
	return data, nil
}
