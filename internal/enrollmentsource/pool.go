package enrollmentsource

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

var ErrAdmission = errors.New("inactive service source admission refused")
var ErrPoolClosed = errors.New("inactive service source pool closed")

// PoolLimits are explicit engineering bounds, never production policy defaults.
// MaxAttempts is permanent for the lifetime of this disconnected pool. No GC,
// eviction, renewal, failed-send refund or cross-restart durability is claimed.
type PoolLimits struct{ MaxSources, MaxConcurrent, MaxPerSource, MaxAttempts int }

// PoolSource is independently supplied config DATA, not installed-policy proof.
// ID is a local alias, never a URL supplied by a reserve request/response.
type PoolSource struct {
	ID        string
	Scope     ServiceScope
	Secret    string
	Allowed   []contract.SourceMapping
	Transport Limits
}

func (PoolSource) String() string   { return "[private source pool config DATA]" }
func (PoolSource) GoString() string { return "[private source pool config DATA]" }

// Pool owns only client/cancellation/admission resources, NOT a Store, installed
// policy, grant, live eligibility evidence or challenge. Copies share lifecycle.
type Pool struct{ *pool }
type pool struct {
	mu       sync.Mutex
	sources  map[string]*poolSource
	attempts map[[32]byte]bool
	limits   PoolLimits
	active   int
	closed   bool
	ctx      context.Context
	cancel   context.CancelFunc
}
type poolSource struct {
	client *Service
	active int
}

func (Pool) String() string   { return "[private inactive bounded service source pool]" }
func (Pool) GoString() string { return "[private inactive bounded service source pool]" }
func NewPool(sources []PoolSource, limits PoolLimits) (*Pool, error) {
	if limits.MaxSources < 1 || limits.MaxSources > 32 || limits.MaxConcurrent < 1 || limits.MaxConcurrent > 256 || limits.MaxPerSource < 1 || limits.MaxPerSource > limits.MaxConcurrent || limits.MaxAttempts < 1 || limits.MaxAttempts > 100000 || len(sources) == 0 || len(sources) > limits.MaxSources {
		return nil, ErrAdmission
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{&pool{sources: map[string]*poolSource{}, attempts: map[[32]byte]bool{}, limits: limits, ctx: ctx, cancel: cancel}}
	good := false
	defer func() {
		if !good {
			p.Close()
		}
	}()
	scopes := map[string]bool{}
	for _, entry := range sources {
		if !identifierPattern.MatchString(entry.ID) || p.sources[entry.ID] != nil || !canonicalBudgetOrigin(entry.Scope.SourceOrigin) {
			return nil, ErrAdmission
		}
		client, err := NewService(entry.Scope, entry.Secret, entry.Allowed, entry.Transport)
		if err != nil {
			return nil, ErrAdmission
		}
		// Different aliases/services/keys must not multiply the same configured source
		// concurrency budget. Key rotation/reconfiguration requires a future actual
		// policy owner, not adding another alias to bypass this pool's ceilings.
		key := entry.Scope.SourceOrigin
		if scopes[key] {
			client.CloseIdleConnections()
			return nil, ErrAdmission
		}
		scopes[key] = true
		p.sources[entry.ID] = &poolSource{client: client}
	}
	good = true
	return p, nil
}
func (p *Pool) Close() {
	if p == nil || p.pool == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.cancel()
	for _, source := range p.sources {
		source.client.CloseIdleConnections()
	}
}

// Reserve admits at most ONE attempted source send for each qualified original
// attempt, even if response is lost/invalid or result saved elsewhere fails.
// Concurrent/capacity refusal before admission does not spend a new attempt.
// All output is observed DATA; this method cannot mint a commit/proof permit.
func (p *Pool) Reserve(ctx context.Context, sourceID string, request contract.SourceReserveRequest, now time.Time) (contract.SourceReserve, error) {
	if p == nil || p.pool == nil || ctx == nil || ctx.Err() != nil || now.IsZero() {
		return contract.SourceReserve{}, ErrAdmission
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return contract.SourceReserve{}, ErrPoolClosed
	}
	selected := p.sources[sourceID]
	if selected == nil {
		p.mu.Unlock()
		return contract.SourceReserve{}, ErrAdmission
	}
	// Do all descriptor/mapping validation before consuming capacity; no HTTP,
	// private credential disclosure or DH can occur during these cheap checks.
	if _, err := contract.ServiceReserveDescriptorBytes(request, selected.client.scope.ServiceID); err != nil {
		p.mu.Unlock()
		return contract.SourceReserve{}, ErrAdmission
	}
	allowed := false
	for _, m := range selected.client.allowed {
		if m == request.Mapping {
			allowed = true
		}
	}
	if !allowed {
		p.mu.Unlock()
		return contract.SourceReserve{}, ErrAdmission
	}
	// Scope by actual configured source + authority, NOT caller alias, selected
	// reserve ID, descriptor or key ID. Another configured service/key cannot
	// create another reserve for the same original source/authority/attempt.
	raw, _ := json.Marshal([]string{selected.client.scope.SourceOrigin, selected.client.scope.PrincipalAuthority, request.Launch.AttemptID})
	attempt := sha256.Sum256(raw)
	if p.attempts[attempt] || len(p.attempts) >= p.limits.MaxAttempts || p.active >= p.limits.MaxConcurrent || selected.active >= p.limits.MaxPerSource {
		p.mu.Unlock()
		return contract.SourceReserve{}, ErrAdmission
	}
	p.attempts[attempt] = true
	p.active++
	selected.active++
	requestCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(p.ctx, cancel)
	p.mu.Unlock()
	data, err := selected.client.Reserve(requestCtx, request, now)
	stop()
	cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active--
	selected.active--
	if p.closed || ctx.Err() != nil {
		return contract.SourceReserve{}, ErrUnavailable
	}
	// Once admitted, even a local pre-send failure cannot be externally labeled
	// retry-safe: one slot stays spent. This conservative API does not recover.
	if err != nil {
		return contract.SourceReserve{}, ErrUnavailable
	}
	return data, nil
}

// A stricter CONFIG admission subset, not normalization of protocol identities.
// Reject aliases instead of silently rewriting exact signed/descriptor fields.
// DNS/CNAME resolution is deliberately absent; global limits still cover names
// that happen to resolve to the same service through separately configured DNS.
func canonicalBudgetOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	if host != strings.ToLower(host) || strings.HasSuffix(host, ".") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n == 443 || strconv.Itoa(n) != port {
			return false
		}
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" || addr.Is4In6() || addr.String() != host {
			return false
		}
		return true
	}
	if strings.Contains(host, ":") || len(host) > 253 {
		return false
	}
	numeric := true
	for _, r := range host {
		if r != '.' && (r < '0' || r > '9') {
			numeric = false
		}
	}
	if numeric {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
				return false
			}
		}
	}
	return true
}
