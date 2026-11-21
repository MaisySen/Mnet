package control

// Private explicit finite DEVELOPMENT middleware. No Server route or production
// policy/default is installed. Existing owning Store operations still decide
// authorization. Admission never constitutes a principal or receipt permit.
import (
	"context"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"io"
	"mime"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

type registrarIngressLimits struct {
	MaxRequests, MaxConcurrent, MaxBodyBytes, MaxStateBytes, MaxEntries int
	Timeout                                                             time.Duration
}
type registrarIngress struct{ *registrarIngressOwner }
type registrarIngressOwner struct {
	mu           sync.Mutex
	store        *Store
	limits       registrarIngressLimits
	next         http.Handler
	used, active int
	closed       bool
	life         context.Context
	cancel       context.CancelFunc
}

func (registrarIngress) String() string   { return "[private finite development ingress]" }
func (registrarIngress) GoString() string { return "[private finite development ingress]" }
func (s *Store) newRegistrarDevelopmentIngress(l registrarIngressLimits, next http.Handler) (*registrarIngress, error) {
	if s == nil || next == nil || l.MaxRequests < 1 || l.MaxRequests > 4096 || l.MaxConcurrent < 1 || l.MaxConcurrent > 16 || l.MaxConcurrent > l.MaxRequests || l.MaxBodyBytes < 1 || l.MaxBodyBytes > 16384 || l.MaxStateBytes < 1 || l.MaxStateBytes > 16<<20 || l.MaxEntries < 1 || l.MaxEntries > 4096 || l.Timeout < time.Millisecond || l.Timeout > time.Minute {
		return nil, errRegistrarEnrollment
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	// ONE retained gate per actual Store lifecycle. Reconstructing a middleware,
	// changing config or making another handle cannot refund finite work budget.
	if s.registrarIngress != nil {
		return nil, errRegistrarEnrollment
	}
	life, cancel := context.WithCancel(context.Background())
	o := &registrarIngressOwner{store: s, limits: l, next: next, life: life, cancel: cancel}
	if err := s.registrarIngressBoundLocked(l); err != nil {
		cancel()
		return nil, err
	}
	s.registrarIngress = o
	return &registrarIngress{o}, nil
}
func (o *registrarIngressOwner) close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closed = true
	o.cancel()
}
func (s *Store) registrarIngressBoundLocked(l registrarIngressLimits) error {
	// O(1) count/file-size checks, then bounded shape walk BEFORE history validation,
	// full-state marshal or source work. No history/record eviction at capacity.
	for _, n := range []int{len(s.data.Networks), len(s.data.Nodes), len(s.data.AuthKeys), len(s.data.NodeTokens), len(s.data.NextIP), len(s.data.Memberships), len(s.data.ApprovalIdentities)} {
		if n > l.MaxEntries {
			return errRegistrarEnrollment
		}
	}
	if r := s.data.Registrar; r != nil && (len(r.Records) > l.MaxEntries || len(r.ConsumedProofs) > l.MaxEntries) {
		return errRegistrarEnrollment
	}
	info, err := os.Lstat(s.ownedPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(l.MaxStateBytes) || singleLink(s.ownedPath, info) != nil {
		return errRegistrarEnrollment
	}
	values, rawBytes := l.MaxEntries*128, l.MaxStateBytes
	if !registrarIngressShape(reflect.ValueOf(s.data), l.MaxEntries, &values, &rawBytes) {
		return errRegistrarEnrollment
	}
	return nil
}

// Bounded preflight of this fixed, acyclic DATA schema. Time values have a
// bounded JSON representation; never inspect their private implementation.
// Counts and raw-string bytes are capped BEFORE full history validation or
// serialization. Exact JSON bytes (including escaping/indentation) remain a
// separate publication check. This is not a streaming serializer or O(1) work.
func registrarIngressShape(v reflect.Value, maxCollection int, values, rawBytes *int) bool {
	*values--
	if *values < 0 {
		return false
	}
	if v.Type() == reflect.TypeOf(time.Time{}) {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer:
		return v.IsNil() || registrarIngressShape(v.Elem(), maxCollection, values, rawBytes)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !registrarIngressShape(v.Field(i), maxCollection, values, rawBytes) {
				return false
			}
		}
	case reflect.String:
		*rawBytes -= v.Len()
		return *rawBytes >= 0
	case reflect.Map:
		if v.Len() > maxCollection || v.Len() > *values/2 {
			return false
		}
		iter := v.MapRange()
		for iter.Next() {
			if !registrarIngressShape(iter.Key(), maxCollection, values, rawBytes) || !registrarIngressShape(iter.Value(), maxCollection, values, rawBytes) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Len() > maxCollection || v.Len() > *values {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !registrarIngressShape(v.Index(i), maxCollection, values, rawBytes) {
				return false
			}
		}
	case reflect.Bool, reflect.Int, reflect.Int64:
	default:
		return false // fail closed if the stored DATA schema gains another kind
	}
	return true
}
func (g *registrarIngress) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	refuse := func(status int) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "{}\n")
	}
	if g == nil || g.registrarIngressOwner == nil || r == nil || r.URL == nil || r.Body == nil {
		refuse(503)
		return
	}
	o := g.registrarIngressOwner
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(o.limits.Timeout)
	if err := controller.SetWriteDeadline(deadline); err != nil {
		refuse(503)
		return
	}
	defer controller.SetWriteDeadline(time.Time{})
	o.mu.Lock()
	if o.closed || o.life.Err() != nil || o.used >= o.limits.MaxRequests || o.active >= o.limits.MaxConcurrent {
		o.mu.Unlock()
		refuse(429)
		return
	}
	o.used++
	o.active++
	o.mu.Unlock()
	defer func() { o.mu.Lock(); o.active--; o.mu.Unlock() }()
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	stop := context.AfterFunc(o.life, cancel)
	defer stop()
	scheme := ""
	switch r.URL.Path {
	case registrar.WorkflowPrefix + "begin", registrar.WorkflowPrefix + "submit":
		scheme = registrar.WorkflowScheme
	case registrar.HistoryPath:
		scheme = registrar.HistoryScheme
	case "/api/mnet-node-experimental-v1/map", "/api/mnet-node-experimental-v1/endpoints":
		scheme = "Mnet-Node-Credential-V1 "
	default:
		refuse(404)
		return
	}
	if r.Method != http.MethodPost || r.TLS == nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" || r.URL.Fragment != "" || r.ContentLength > int64(o.limits.MaxBodyBytes) || len(r.Header.Values("Authorization")) != 1 || len(r.Header.Values("Content-Type")) != 1 || len(r.Header.Get("Content-Type")) > 64 || len(r.Header.Values("Content-Encoding")) != 0 || len(r.Header.Values("Cookie")) != 0 || len(r.Header.Values("Origin")) != 0 || len(r.Header.Values("Referer")) != 0 {
		refuse(400)
		return
	}
	auth := r.Header.Get("Authorization")
	if len(auth) != len(scheme)+43 || !strings.HasPrefix(auth, scheme) || !registrarCredentialSecret(strings.TrimPrefix(auth, scheme)) {
		refuse(403)
		return
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) > 1 || (len(params) == 1 && params["charset"] != "utf-8") {
		refuse(400)
		return
	}
	// A real HTTP connection must support a bounded read. Unsupported custom
	// writers fail closed, no unbounded goroutine/body reader is started.
	if err := controller.SetReadDeadline(deadline); err != nil {
		refuse(503)
		return
	}
	defer controller.SetReadDeadline(time.Time{})
	raw, err := io.ReadAll(io.LimitReader(r.Body, int64(o.limits.MaxBodyBytes)+1))
	if err != nil || len(raw) == 0 || len(raw) > o.limits.MaxBodyBytes || ctx.Err() != nil {
		refuse(400)
		return
	}
	if !o.store.mu.TryLock() {
		refuse(503)
		return
	}
	healthy := o.store.healthLocked()
	if healthy == nil {
		healthy = o.store.registrarIngressBoundLocked(o.limits)
	}
	o.store.mu.Unlock()
	if healthy != nil || ctx.Err() != nil {
		refuse(503)
		return
	}
	o.mu.Lock()
	closed := o.closed
	o.mu.Unlock()
	if closed || o.life.Err() != nil {
		refuse(503)
		return
	}
	// Fresh reader only for the bounded validated envelope. Inner owning handler
	// still must strict-parse purpose-specific JSON and run actual authentication.
	r = r.Clone(ctx)
	r.Body = io.NopCloser(strings.NewReader(string(raw)))
	r.ContentLength = int64(len(raw))
	o.next.ServeHTTP(w, r)
}
