package enrollmentregistrar

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/enrollmentidentity"
	"io"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"
)

const HistoryScheme = "Mnet-Enrollment-History-V1 "
const HistoryPath = "/api/mnet-enrollment-history-experimental-v1/receipt"

type HistoryClient struct{ *historyOwner }
type historyOwner struct {
	mu             sync.Mutex
	origin, secret string
	challenge      contract.Challenge
	client         *http.Client
	transport      *http.Transport
	lifetime       context.Context
	cancel         context.CancelFunc
	spent, closed  bool
}

func (HistoryClient) String() string   { return "[private one-shot history channel]" }
func (HistoryClient) GoString() string { return "[private one-shot history channel]" }

// NewHistory accepts independent original pinned context, not a response-selected
// reference. Caller must own provenance/current policy before disclosure. No live
// challenge/source/lease requirement: expired/deleted history remains history.
// Constructor and public receipt grammar alone create no authentication permit.
func NewHistory(origin string, c contract.Challenge, secret string, timeout time.Duration) (*HistoryClient, error) {
	if !enrollmentidentity.ValidOrigin(origin) || origin != c.Binding.RegistrarOrigin || timeout < time.Millisecond || timeout > time.Minute {
		return nil, ErrInvalid
	}
	if _, err := contract.Encode(c); err != nil {
		return nil, ErrInvalid
	}
	digest, err := contract.NativeCredentialSecretDigest(secret)
	if err != nil || subtle.ConstantTimeCompare([]byte(digest), []byte(c.Binding.CredentialDigest)) != 1 {
		return nil, ErrInvalid
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: timeout}).DialContext, ForceAttemptHTTP2: false, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, MaxResponseHeaderBytes: 16384, MaxConnsPerHost: 1, DisableCompression: true}
	life, cancel := context.WithCancel(context.Background())
	return &HistoryClient{&historyOwner{origin: origin, secret: secret, challenge: c, transport: tr, lifetime: life, cancel: cancel, client: &http.Client{Transport: tr, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}}, nil
}
func (c *HistoryClient) Close() {
	if c == nil || c.historyOwner == nil {
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
	c.challenge = contract.Challenge{}
}

// Receipt performs one read-only lookup. Any failure stays unknown, not absence
// or permission to submit/reserve again. No automatic retry or response replay.
func (c *HistoryClient) Receipt(ctx context.Context) (contract.HistoryReceiptWire, error) {
	var empty contract.HistoryReceiptWire
	if c == nil || c.historyOwner == nil || ctx == nil || ctx.Err() != nil {
		return empty, ErrInvalid
	}
	c.mu.Lock()
	if c.spent || c.closed {
		c.mu.Unlock()
		return empty, ErrInvalid
	}
	c.spent = true
	origin, secret, pinned, life := c.origin, c.secret, c.challenge, c.lifetime
	c.mu.Unlock()
	raw, _ := json.Marshal(contract.HistoryRequestWire{Version: 1, Challenge: pinned})
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(life, cancel)
	defer stop()
	req, err := http.NewRequestWithContext(child, http.MethodPost, origin+HistoryPath, bytes.NewReader(raw))
	if err != nil {
		return empty, ErrUnavailable
	}
	req.GetBody = nil
	req.Header.Set("Authorization", HistoryScheme+secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Pragma", "no-cache")
	resp, err := c.client.Do(req)
	if err != nil {
		return empty, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength > contract.ProofWireMaxBytes || resp.Header.Get("Content-Encoding") != "" || len(resp.Header.Values("Content-Type")) != 1 || len(resp.Header.Values("Cache-Control")) != 1 || resp.Header.Get("Cache-Control") != "no-store" {
		return empty, ErrUnavailable
	}
	typ, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" || len(params) > 1 || len(params) == 1 && params["charset"] != "utf-8" {
		return empty, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, contract.ProofWireMaxBytes+1))
	if err != nil || len(body) > contract.ProofWireMaxBytes {
		return empty, ErrUnavailable
	}
	result, err := contract.DecodeHistoryReceiptWire(body, pinned)
	if err != nil {
		return empty, ErrUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || ctx.Err() != nil {
		return empty, ErrUnavailable
	}
	return result, nil
}
