// Package enrollmentregistrar implements an INACTIVE experimental exact-origin
// bootstrap transport. No key signing, source verification, route, credential
// installation, receipt/current-control API, Store permit or app caller exists.
package enrollmentregistrar

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/enrollmentidentity"
)

var ErrInvalid = errors.New("invalid inactive registrar bootstrap request")
var ErrUnavailable = errors.New("registrar bootstrap unavailable; remote outcome may be unknown; attempt remains spent")

// Limits explicitly bound ENGINEERING transport, not source eligibility/lease.
type Limits struct{ Timeout, MaxChallengeLifetime time.Duration }

// Client copies share one owner, one challenge request and one submission. No
// production caller may reconstruct a client to erase unknown durable history.
type Client struct{ *owner }
type owner struct {
	mu                                      sync.Mutex
	expected                                contract.Binding
	secret, origin                          string
	limits                                  Limits
	http                                    *http.Client
	transport                               *http.Transport
	challengeStarted, submitStarted, closed bool
	pinned                                  *contract.Challenge
	lifetime                                context.Context
	cancel                                  context.CancelFunc
}

func (Client) String() string   { return "[private inactive exact-origin registrar bootstrap client]" }
func (Client) GoString() string { return "[private inactive exact-origin registrar bootstrap client]" }

const scheme = "Mnet-Enrollment-Bootstrap-V4 "
const prefix = "/api/mnet-registrar-experimental-v4/"

func New(origin string, expected contract.Binding, secret string, limits Limits) (*Client, error) {
	if !enrollmentidentity.ValidOrigin(origin) || origin != expected.RegistrarOrigin || limits.Timeout <= 0 || limits.Timeout > time.Minute || limits.MaxChallengeLifetime <= 0 || limits.MaxChallengeLifetime > 15*time.Minute {
		return nil, ErrInvalid
	}
	digest, err := contract.NativeCredentialSecretDigest(secret)
	if err != nil {
		return nil, ErrInvalid
	}
	if subtle.ConstantTimeCompare([]byte(digest), []byte(expected.CredentialDigest)) != 1 {
		return nil, ErrInvalid
	}
	// Shape only, not an owned challenge, proof or source provenance. All values
	// below are caller DATA and are never accepted as a response or sent as MAC.
	probe := contract.Challenge{Binding: expected, ID: expected.AttemptID, Nonce: expected.SessionBinding, EphemeralPublicKey: expected.DevicePublicKey, ExpiresAtMS: expected.AccessUntilMS}
	if _, err := contract.Encode(probe); err != nil {
		return nil, ErrInvalid
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: limits.Timeout}).DialContext, ForceAttemptHTTP2: false, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}, TLSHandshakeTimeout: limits.Timeout, ResponseHeaderTimeout: limits.Timeout, MaxResponseHeaderBytes: 16384, MaxConnsPerHost: 1, DisableCompression: true}
	life, cancel := context.WithCancel(context.Background())
	return &Client{&owner{expected: expected, secret: secret, origin: origin, limits: limits, transport: tr, http: &http.Client{Transport: tr, Timeout: limits.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, lifetime: life, cancel: cancel}}, nil
}
func (c *Client) Close() {
	if c == nil || c.owner == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.cancel()
	c.transport.CloseIdleConnections()
	c.secret = ""
	c.pinned = nil
}
func (c *Client) current(now time.Time) bool {
	return now.UnixMilli() >= c.expected.CheckedAtMS && now.UnixMilli() < c.expected.AccessUntilMS && now.UnixMilli() < c.expected.ReserveDeadlineMS
}

// Challenge spends request slot BEFORE I/O. No automatic retry or new lifetime.
// Caller must own durable pre-saved attempt intent before invoking; this helper
// cannot prove it, and its constructor/pinned pointer is NOT that evidence.
func (c *Client) Challenge(ctx context.Context) (contract.Challenge, error) {
	if c == nil || c.owner == nil || ctx == nil || ctx.Err() != nil {
		return contract.Challenge{}, ErrInvalid
	}
	c.mu.Lock()
	if c.closed || c.challengeStarted || !c.current(time.Now()) {
		c.mu.Unlock()
		return contract.Challenge{}, ErrInvalid
	}
	c.challengeStarted = true
	expected := c.expected
	c.mu.Unlock()
	payload := struct {
		Version   int    `json:"version"`
		AttemptID string `json:"attempt_id"`
		HandoffID string `json:"handoff_id"`
		ReserveID string `json:"reserve_id"`
	}{4, expected.AttemptID, expected.HandoffID, expected.ReserveID}
	raw, err := c.request(ctx, "challenge", payload)
	if err != nil {
		return contract.Challenge{}, err
	}
	challenge, err := contract.DecodeChallengeWire(raw)
	if err != nil {
		return contract.Challenge{}, ErrUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.closed || !c.current(now) || challenge.Binding != c.expected || challenge.ExpiresAtMS <= now.UnixMilli() || challenge.ExpiresAtMS-now.UnixMilli() > c.limits.MaxChallengeLifetime.Milliseconds() || challenge.ExpiresAtMS-challenge.Binding.CheckedAtMS > c.limits.MaxChallengeLifetime.Milliseconds() {
		return contract.Challenge{}, ErrUnavailable
	}
	copy := challenge
	c.pinned = &copy
	return challenge, nil
}

// Submit sends only owned challenge ID and caller MAC, never caller Binding.
// Confirmation DATA does NOT prove durable commit/enrollment/current access.
func (c *Client) Submit(ctx context.Context, id string, response [32]byte) (contract.ConfirmationWire, error) {
	if c == nil || c.owner == nil || ctx == nil || ctx.Err() != nil {
		return contract.ConfirmationWire{}, ErrInvalid
	}
	c.mu.Lock()
	if c.closed || c.submitStarted || c.pinned == nil || id != c.pinned.ID || !c.current(time.Now()) || time.Now().UnixMilli() >= c.pinned.ExpiresAtMS {
		c.mu.Unlock()
		return contract.ConfirmationWire{}, ErrInvalid
	}
	payload := contract.ResponseWire{Version: 4, ChallengeID: c.pinned.ID, ResponseMAC: base64.RawURLEncoding.EncodeToString(response[:])}
	raw, _ := json.Marshal(payload)
	if _, err := contract.DecodeResponseWire(raw); err != nil {
		c.mu.Unlock()
		return contract.ConfirmationWire{}, ErrInvalid
	}
	c.submitStarted = true
	c.mu.Unlock()
	body, err := c.request(ctx, "response", payload)
	if err != nil {
		return contract.ConfirmationWire{}, err
	}
	confirmation, err := contract.DecodeConfirmationWire(body)
	if err != nil {
		return contract.ConfirmationWire{}, ErrUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.pinned == nil || !c.current(time.Now()) || time.Now().UnixMilli() >= c.pinned.ExpiresAtMS || confirmation.ChallengeID != c.pinned.ID {
		return contract.ConfirmationWire{}, ErrUnavailable
	}
	return confirmation, nil
}
func (c *Client) request(ctx context.Context, path string, payload any) ([]byte, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrUnavailable
	}
	secret, origin, life := c.secret, c.origin, c.lifetime
	c.mu.Unlock()
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > contract.ProofWireMaxBytes {
		return nil, ErrUnavailable
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(life, cancel)
	defer stop()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, origin+prefix+path, bytes.NewReader(raw))
	if err != nil {
		return nil, ErrUnavailable
	}
	req.GetBody = nil
	req.Header.Set("Authorization", scheme+secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Pragma", "no-cache")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > contract.ProofWireMaxBytes || resp.Header.Get("Content-Encoding") != "" || len(resp.Header.Values("Content-Type")) != 1 || len(resp.Header.Values("Cache-Control")) != 1 || resp.Header.Get("Cache-Control") != "no-store" {
		return nil, ErrUnavailable
	}
	typ, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" || len(params) > 1 || (len(params) == 1 && params["charset"] != "utf-8") {
		return nil, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, contract.ProofWireMaxBytes+1))
	if err != nil || len(body) > contract.ProofWireMaxBytes {
		return nil, ErrUnavailable
	}
	return body, nil
}
