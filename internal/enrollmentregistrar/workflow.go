package enrollmentregistrar

// Distinct unregistered workflow-v1 native transport. No key access, source
// permit, State writer, enrollment activation or retry/recovery policy exists.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

type WorkflowLimits struct {
	Timeout              time.Duration
	MaxChallengeLifetime time.Duration
	MaxReserveLifetime   time.Duration
	Timing               contract.FixtureTiming
}
type WorkflowClient struct{ *workflowOwner }
type workflowOwner struct {
	mu                         sync.Mutex
	own                        contract.SourceExchange
	request                    contract.SourceReserveRequest
	service, secret            string
	limits                     WorkflowLimits
	client                     *http.Client
	transport                  *http.Transport
	lifetime                   context.Context
	cancel                     context.CancelFunc
	started, submitted, closed bool
	pinned                     *contract.Challenge
}

func (WorkflowClient) String() string   { return "[private experimental workflow transport]" }
func (WorkflowClient) GoString() string { return "[private experimental workflow transport]" }

const WorkflowScheme = "Mnet-Enrollment-Workflow-V1 "
const WorkflowPrefix = "/api/mnet-enrollment-workflow-experimental-v1/"

func ValidWorkflowLimits(l WorkflowLimits) bool {
	return l.Timeout >= time.Millisecond && l.Timeout <= time.Minute && l.MaxChallengeLifetime >= time.Millisecond && l.MaxChallengeLifetime <= time.Minute && l.MaxReserveLifetime >= time.Millisecond && l.MaxReserveLifetime <= 2*time.Minute && l.Timing.MaxObservationAgeMS > 0 && l.Timing.MaxObservationAgeMS <= 60000 && l.Timing.MaxLeaseMS > 0 && l.Timing.MaxLeaseMS <= 900000 && l.Timing.ClockUncertaintyMS >= 0 && l.Timing.ClockUncertaintyMS < l.MaxChallengeLifetime.Milliseconds() && l.Timing.ClockUncertaintyMS < l.Timing.MaxLeaseMS
}
func NewWorkflow(own contract.SourceExchange, service, reserveID, secret string, l WorkflowLimits) (*WorkflowClient, error) {
	if !ValidWorkflowLimits(l) {
		return nil, ErrInvalid
	}
	raw, err := json.Marshal(own)
	if err != nil {
		return nil, ErrInvalid
	}
	checked, err := contract.DecodeSourceExchange(raw, own.Launch, own.Provenance.PrincipalAuthority, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, time.Now())
	if err != nil || checked != own {
		return nil, ErrInvalid
	}
	digest, err := contract.NativeCredentialSecretDigest(secret)
	if err != nil || subtle.ConstantTimeCompare([]byte(digest), []byte(own.Launch.CredentialDigest)) != 1 {
		return nil, ErrInvalid
	}
	request := contract.SourceReserveRequest{Ticket: own.Ticket, HandoffID: own.ID, ReserveID: reserveID, Launch: own.Launch, Mapping: own.Mapping}
	if _, err := contract.ServiceReserveDescriptorBytes(request, service); err != nil {
		return nil, ErrInvalid
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: l.Timeout}).DialContext, ForceAttemptHTTP2: false, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}, TLSHandshakeTimeout: l.Timeout, ResponseHeaderTimeout: l.Timeout, MaxResponseHeaderBytes: 16384, MaxConnsPerHost: 1, DisableCompression: true}
	life, cancel := context.WithCancel(context.Background())
	return &WorkflowClient{&workflowOwner{own: own, request: request, service: service, secret: secret, limits: l, transport: tr, client: &http.Client{Transport: tr, Timeout: l.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, lifetime: life, cancel: cancel}}, nil
}
func (c *WorkflowClient) Close() {
	if c == nil || c.workflowOwner == nil {
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
	c.own = contract.SourceExchange{}
	c.request = contract.SourceReserveRequest{}
	c.pinned = nil
}

// CheckWorkflowChallenge compares every non-time field against independently
// saved own-PKCE + configured service/reserve. Only bounded registrar timing is
// selected by the response. This is DATA validation, not a key-use permit.
func CheckWorkflowChallenge(c contract.Challenge, own contract.SourceExchange, service, reserveID string, l WorkflowLimits, now time.Time) error {
	if !ValidWorkflowLimits(l) || now.IsZero() {
		return ErrInvalid
	}
	expected, err := OwnWorkflowBindingDATA(own, service, reserveID, c.Binding.CheckedAtMS, c.Binding.ReserveDeadlineMS, c.Binding.AccessUntilMS)
	if err != nil {
		return ErrInvalid
	}
	b := c.Binding
	if b != expected || contract.CheckFixtureTime(c, now.UnixMilli(), l.Timing) != nil || b.ReserveDeadlineMS-b.CheckedAtMS > l.MaxReserveLifetime.Milliseconds() || c.ExpiresAtMS-b.CheckedAtMS > l.MaxChallengeLifetime.Milliseconds() {
		return ErrInvalid
	}
	return nil
}

// OwnWorkflowBindingDATA reconstructs identity exclusively from independent own
// context. Only explicit time metadata may come from a previously checked
// challenge. This helper authenticates nothing and is not key-use admission.
func OwnWorkflowBindingDATA(own contract.SourceExchange, service, reserveID string, checkedAt, reserveDeadline, accessUntil int64) (contract.Binding, error) {
	descriptor, err := contract.SourceDescriptorDigest(own, service, reserveID)
	if err != nil {
		return contract.Binding{}, ErrInvalid
	}
	p, m := own.Provenance, own.Mapping
	return contract.Binding{PrincipalAuthority: p.PrincipalAuthority, PrincipalNamespace: p.PrincipalNamespace, PrincipalID: p.PrincipalID, SessionBinding: p.SessionBinding, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, ServiceID: service, WorkspaceID: m.WorkspaceID, NetworkID: m.NetworkID, MappingRevision: m.MappingRevision, HandoffID: own.ID, AttemptID: own.Launch.AttemptID, ReserveID: reserveID, DescriptorDigest: descriptor, DescriptorEncodingVersion: 1, DevicePublicKey: own.Launch.DevicePublicKey, CredentialDigest: own.Launch.CredentialDigest, CheckedAtMS: checkedAt, ReserveDeadlineMS: reserveDeadline, AccessUntilMS: accessUntil}, nil
}

func (c *WorkflowClient) Begin(ctx context.Context) (contract.Challenge, error) {
	if c == nil || c.workflowOwner == nil || ctx == nil || ctx.Err() != nil {
		return contract.Challenge{}, ErrInvalid
	}
	c.mu.Lock()
	if c.closed || c.started {
		c.mu.Unlock()
		return contract.Challenge{}, ErrInvalid
	}
	c.started = true
	own, request, service, l := c.own, c.request, c.service, c.limits
	c.mu.Unlock()
	raw, err := c.send(ctx, "begin", contract.WorkflowBeginWire{Version: 1, Request: request})
	if err != nil {
		return contract.Challenge{}, err
	}
	got, err := contract.DecodeChallengeWire(raw)
	if err != nil || CheckWorkflowChallenge(got, own, service, request.ReserveID, l, time.Now()) != nil {
		return contract.Challenge{}, ErrUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || ctx.Err() != nil {
		return contract.Challenge{}, ErrUnavailable
	}
	copy := got
	c.pinned = &copy
	return got, nil
}

// Submit accepts only SAME owned challenge, no caller binding. Receipt delivery
// is authenticated to exact configured TLS origin, never a public codec proof.
// Confirmation still needs the native proof owner's independent MAC check.
func (c *WorkflowClient) SubmitWorkflow(ctx context.Context, id string, response [32]byte) (contract.WorkflowReceiptWire, error) {
	var empty contract.WorkflowReceiptWire
	if c == nil || c.workflowOwner == nil || ctx == nil || ctx.Err() != nil {
		return empty, ErrInvalid
	}
	c.mu.Lock()
	if c.closed || c.submitted || c.pinned == nil || id != c.pinned.ID || CheckWorkflowChallenge(*c.pinned, c.own, c.service, c.request.ReserveID, c.limits, time.Now()) != nil {
		c.mu.Unlock()
		return empty, ErrInvalid
	}
	pinned := *c.pinned
	c.submitted = true
	c.mu.Unlock()
	payload := contract.WorkflowSubmitWire{Version: 1, AttemptID: pinned.Binding.AttemptID, ChallengeID: id, ResponseMAC: base64.RawURLEncoding.EncodeToString(response[:])}
	encoded, _ := json.Marshal(payload)
	if _, err := contract.DecodeWorkflowSubmitWire(encoded); err != nil {
		return empty, ErrInvalid
	}
	raw, err := c.send(ctx, "submit", payload)
	if err != nil {
		return empty, err
	}
	receipt, err := contract.DecodeWorkflowReceiptWire(raw, pinned)
	if err != nil {
		return empty, ErrUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || ctx.Err() != nil {
		return empty, ErrUnavailable
	}
	return receipt, nil
}
func (c *WorkflowClient) send(ctx context.Context, path string, payload any) ([]byte, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrUnavailable
	}
	origin, secret, life := c.own.Mapping.RegistrarOrigin, c.secret, c.lifetime
	c.mu.Unlock()
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > contract.ProofWireMaxBytes {
		return nil, ErrUnavailable
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(life, cancel)
	defer stop()
	req, err := http.NewRequestWithContext(child, http.MethodPost, origin+WorkflowPrefix+path, bytes.NewReader(raw))
	if err != nil {
		return nil, ErrUnavailable
	}
	req.GetBody = nil
	req.Header.Set("Authorization", WorkflowScheme+secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Pragma", "no-cache")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > contract.ProofWireMaxBytes || resp.Header.Get("Content-Encoding") != "" || len(resp.Header.Values("Content-Type")) != 1 || len(resp.Header.Values("Cache-Control")) != 1 || resp.Header.Get("Cache-Control") != "no-store" {
		return nil, ErrUnavailable
	}
	typ, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" || len(params) > 1 || len(params) == 1 && params["charset"] != "utf-8" {
		return nil, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, contract.ProofWireMaxBytes+1))
	if err != nil || len(body) > contract.ProofWireMaxBytes {
		return nil, ErrUnavailable
	}
	return body, nil
}

// Hash only pins caller MAC bytes in a journal without retaining another raw
// response copy. It never authenticates a proof, receipt or permission.
func WorkflowResponseDigest(response [32]byte) string {
	sum := sha256.Sum256(response[:])
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
