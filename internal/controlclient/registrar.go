package controlclient

// INACTIVE dedicated registrar-node transport. It has no legacy Join/rejoin,
// relay, installer, production daemon entrypoint or registered server endpoint.
// Private native sole-State callers independently enforce owner/policy/pending. Constructor
// document is caller DATA, not authenticated receipt provenance or authority.
import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/netmap"
)

var ErrRegistrarInvalid = errors.New("invalid inactive registrar-node request")
var ErrRegistrarUnavailable = errors.New("registrar-node request unavailable; remote update outcome may be unknown")

// Never alias legacy ErrUnauthorized: failure must NOT initiate legacy rejoin.
var ErrRegistrarDenied = errors.New("registrar-node access ended; legacy rejoin is forbidden")

type RegistrarLimits struct {
	Timeout                    time.Duration
	MaxConcurrent, MaxRequests int
}
type RegistrarClient struct{ *registrarClientOwner }
type registrarClientOwner struct {
	mu             sync.Mutex
	document       contract.NativeCredentialDocument
	limits         RegistrarLimits
	origin         string
	http           *http.Client
	transport      *http.Transport
	lifetime       context.Context
	cancel         context.CancelFunc
	used, inflight int
	closed, denied bool
}

func (RegistrarClient) String() string   { return "[private inactive registrar-node client]" }
func (RegistrarClient) GoString() string { return "[private inactive registrar-node client]" }

const registrarNodePath = "/api/mnet-node-experimental-v1/"
const registrarNodeScheme = "Mnet-Node-Credential-V1 "

// Explicit immutable document copy. No file/journal read or current installed
// policy check. Actual private native owner supplies those before invocation.
func NewRegistrarClient(origin string, document contract.NativeCredentialDocument, limits RegistrarLimits) (*RegistrarClient, error) {
	if contract.CheckNativeCredentialDocument(document) != nil || origin != document.Receipt.RegistrarOrigin || limits.Timeout <= 0 || limits.Timeout > time.Minute || limits.MaxConcurrent < 1 || limits.MaxConcurrent > 4 || limits.MaxRequests < limits.MaxConcurrent || limits.MaxRequests > 256 {
		return nil, ErrRegistrarInvalid
	}
	now := time.Now().UnixMilli()
	if now < document.Receipt.CommittedAtMS || now >= document.Receipt.AccessExpiresAtMS {
		return nil, ErrRegistrarInvalid
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: limits.Timeout}).DialContext, ForceAttemptHTTP2: false, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}, TLSHandshakeTimeout: limits.Timeout, ResponseHeaderTimeout: limits.Timeout, MaxResponseHeaderBytes: 16384, MaxConnsPerHost: limits.MaxConcurrent, DisableCompression: true}
	life, cancel := context.WithDeadline(context.Background(), time.UnixMilli(document.Receipt.AccessExpiresAtMS))
	return &RegistrarClient{&registrarClientOwner{document: document, origin: origin, limits: limits, transport: tr, http: &http.Client{Transport: tr, Timeout: limits.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, lifetime: life, cancel: cancel}}, nil
}
func (c *RegistrarClient) Close() {
	if c == nil || c.registrarClientOwner == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked(false)
}
func (c *RegistrarClient) closeLocked(denied bool) {
	c.denied = c.denied || denied
	c.closed = true
	c.cancel()
	c.transport.CloseIdleConnections()
	c.document = contract.NativeCredentialDocument{}
}

type registrarNodeRequest struct {
	Version       int                `json:"version"`
	Receipt       contract.ReceiptV2 `json:"receipt"`
	ReceiptDigest string             `json:"receipt_digest"`
	Endpoints     []string           `json:"endpoints"`
}
type registrarMapResponse struct {
	Version       int         `json:"version"`
	ReceiptDigest string      `json:"receipt_digest"`
	Map           *netmap.Map `json:"map"`
}
type registrarEndpointsResponse struct {
	Version       int    `json:"version"`
	ReceiptDigest string `json:"receipt_digest"`
	Updated       bool   `json:"updated"`
}

func (c *RegistrarClient) CurrentMap(ctx context.Context) (*netmap.Map, error) {
	body, doc, err := c.registrarRequest(ctx, "map", nil)
	if err != nil {
		return nil, err
	}
	var response registrarMapResponse
	if !registrarResponseJSON(body, &response) || response.Version != 1 || response.ReceiptDigest != doc.ReceiptDigest || response.Map == nil {
		return nil, ErrRegistrarUnavailable
	}
	m := response.Map
	r := doc.Receipt
	if m.RelayURL != "" || m.Version < 0 || m.Network.ID != r.NetworkID || m.Self.ID != r.NodeID || m.Self.NetworkID != r.NetworkID || m.Self.PubKey != r.DevicePublicKey || !m.Self.Approved || m.Self.KeyExpiry == nil || m.Self.KeyExpiry.UnixMilli() != r.AccessExpiresAtMS || m.Self.ExitNode || len(m.Self.Routes) != 0 || m.Self.User != "" || len(m.Peers) > 4096 {
		return nil, ErrRegistrarUnavailable
	}
	if err := c.registrarCurrent(); err != nil {
		return nil, err
	}
	return m, nil
}
func (c *RegistrarClient) RegistrarEndpoints(ctx context.Context, endpoints []string) error {
	if len(endpoints) > 64 {
		return ErrRegistrarInvalid
	}
	copy := append([]string{}, endpoints...)
	for _, endpoint := range copy {
		addr, err := netip.ParseAddrPort(endpoint)
		if err != nil || addr.Port() == 0 || addr.Addr().Zone() != "" || addr.Addr().Is4In6() || addr.String() != endpoint {
			return ErrRegistrarInvalid
		}
	}
	body, doc, err := c.registrarRequest(ctx, "endpoints", copy)
	if err != nil {
		return err
	}
	var response registrarEndpointsResponse
	if !registrarResponseJSON(body, &response) || response.Version != 1 || response.ReceiptDigest != doc.ReceiptDigest || !response.Updated {
		return ErrRegistrarUnavailable
	}
	return c.registrarCurrent()
}
func (c *RegistrarClient) registrarCurrent() error {
	if c == nil || c.registrarClientOwner == nil {
		return ErrRegistrarInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.denied {
		return ErrRegistrarDenied
	}
	if c.closed || c.lifetime.Err() != nil || time.Now().UnixMilli() < c.document.Receipt.CommittedAtMS {
		return ErrRegistrarUnavailable
	}
	return nil
}
func (c *RegistrarClient) registrarRequest(ctx context.Context, path string, eps []string) ([]byte, contract.NativeCredentialDocument, error) {
	empty := contract.NativeCredentialDocument{}
	if c == nil || c.registrarClientOwner == nil || ctx == nil || ctx.Err() != nil {
		return nil, empty, ErrRegistrarInvalid
	}
	c.mu.Lock()
	if c.denied {
		c.mu.Unlock()
		return nil, empty, ErrRegistrarDenied
	}
	if c.closed || c.lifetime.Err() != nil || time.Now().UnixMilli() < c.document.Receipt.CommittedAtMS || c.used >= c.limits.MaxRequests || c.inflight >= c.limits.MaxConcurrent {
		c.mu.Unlock()
		return nil, empty, ErrRegistrarInvalid
	}
	c.used++
	c.inflight++
	doc, origin, life := c.document, c.origin, c.lifetime
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.inflight--; c.mu.Unlock() }()
	raw, _ := json.Marshal(registrarNodeRequest{Version: 1, Receipt: doc.Receipt, ReceiptDigest: doc.ReceiptDigest, Endpoints: eps})
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(life, cancel)
	defer stop()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, origin+registrarNodePath+path, bytes.NewReader(raw))
	if err != nil {
		return nil, empty, ErrRegistrarUnavailable
	}
	req.GetBody = nil
	req.Header.Set("Authorization", registrarNodeScheme+doc.Secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Pragma", "no-cache")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, empty, ErrRegistrarUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		c.mu.Lock()
		c.closeLocked(true)
		c.mu.Unlock()
		return nil, empty, ErrRegistrarDenied
	}
	if response.StatusCode != 200 || response.ContentLength > 4<<20 || response.Header.Get("Content-Encoding") != "" || len(response.Header.Values("Content-Type")) != 1 || len(response.Header.Values("Cache-Control")) != 1 || response.Header.Get("Cache-Control") != "no-store" {
		return nil, empty, ErrRegistrarUnavailable
	}
	typ, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" || len(params) > 1 || (len(params) == 1 && params["charset"] != "utf-8") {
		return nil, empty, ErrRegistrarUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, empty, ErrRegistrarUnavailable
	}
	if err := c.registrarCurrent(); err != nil {
		return nil, empty, err
	}
	return body, doc, nil
}

// Exact JSON fields, recursive duplicate/case/unknown refusal. Legacy map
// nullable slices/optional pointers are preserved; required envelope cannot be
// null. Go timestamp and surrogate semantics remain DATA, not authenticated bytes.
func registrarResponseJSON(raw []byte, out any) bool {
	if len(raw) == 0 || len(raw) > 4<<20 || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !registrarShape(d, reflect.TypeOf(out).Elem(), 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
func registrarShape(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 12 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	if t.Kind() == reflect.Pointer {
		if token == nil {
			return true
		}
		t = t.Elem()
	}
	if t == reflect.TypeFor[time.Time]() {
		s, ok := token.(string)
		if !ok {
			return false
		}
		_, err := time.Parse(time.RFC3339Nano, s)
		return err == nil
	}
	switch t.Kind() {
	case reflect.Struct:
		if token != json.Delim('{') {
			return false
		}
		fields := map[string]reflect.Type{}
		required := map[string]bool{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			tag := strings.Split(field.Tag.Get("json"), ",")
			if tag[0] == "-" || field.PkgPath != "" {
				continue
			}
			fields[tag[0]] = field.Type
			required[tag[0]] = len(tag) == 1 || tag[1] != "omitempty"
		}
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			field, ok := fields[name]
			if !ok {
				return false
			}
			seen[name] = true
			if !registrarShape(d, field, depth+1) {
				return false
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return false
		}
		for name, needed := range required {
			if needed && !seen[name] {
				return false
			}
		}
		return true
	case reflect.Slice:
		if token == nil {
			return true
		}
		if token != json.Delim('[') {
			return false
		}
		count := 0
		for d.More() {
			count++
			if count > 8192 || !registrarShape(d, t.Elem(), depth+1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	case reflect.String:
		_, ok := token.(string)
		return ok
	case reflect.Bool:
		_, ok := token.(bool)
		return ok
	case reflect.Int, reflect.Int64:
		number, ok := token.(json.Number)
		if !ok {
			return false
		}
		_, err := number.Int64()
		return err == nil
	}
	return false
}
