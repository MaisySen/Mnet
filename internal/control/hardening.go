package control

// Hardening: security headers, per-IP rate limits on the open
// endpoints, and an append-only audit log of administrative and
// join events.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// securityHeaders sets browser hardening headers on every response.
// API responses additionally deny all content loading; the dashboard
// pages keep inline scripts working with a self-only policy.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/admin/api/") {
			h.Set("Content-Security-Policy", "default-src 'none'")
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// ipLimiters keeps a token bucket per client IP, expiring buckets
// idle for an hour so the map does not grow forever.
type ipLimiters struct {
	mu        sync.Mutex
	m         map[string]*ipBucket
	r         rate.Limit
	b         int
	now       func() time.Time
	onLimit   func() // optional, called on every 429
	address   func(*http.Request) string
	nextSweep time.Time
}

type ipBucket struct {
	lim  *rate.Limiter
	last time.Time
}

func newIPLimiters(perMinute, burst int) *ipLimiters {
	return &ipLimiters{
		m:   make(map[string]*ipBucket),
		r:   rate.Limit(float64(perMinute) / 60),
		b:   burst,
		now: time.Now,
	}
}

// allow reports whether this IP may proceed right now.
func (l *ipLimiters) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.nextSweep) {
		for ip, bkt := range l.m {
			if now.Sub(bkt.last) > time.Hour {
				delete(l.m, ip)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}
	bkt, ok := l.m[ip]
	if !ok {
		// Fail closed under address churn instead of evicting active buckets
		// (which would let a caller reset its admission budget).
		if len(l.m) >= maxIPBuckets {
			return false
		}
		bkt = &ipBucket{lim: rate.NewLimiter(l.r, l.b)}
		l.m[ip] = bkt
	}
	bkt.last = now
	return bkt.lim.AllowN(now, 1)
}

// limit wraps h with a 429 when the client IP exceeds the bucket.
func (l *ipLimiters) limit(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address := clientIP
		if l.address != nil {
			address = l.address
		}
		if !l.allow(address(r)) {
			if l.onLimit != nil {
				l.onLimit()
			}
			w.Header().Set("Retry-After", "60")
			writeErr(w, http.StatusTooManyRequests, "rate limited; retry later")
			return
		}
		h(w, r)
	}
}

// A bounded table and amortized expiry scan keep unique-address floods
// from growing memory without limit or turning each request into O(n).
const maxIPBuckets = 10000

// clientIP never trusts a forwarding header without an explicit proxy policy.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// proxyAddress trusts X-Forwarded-For only from a configured direct peer.
// Walk right to left, stopping at the first untrusted hop. A forged prefix
// supplied by that hop is never allowed to choose the limiter/audit address.
func proxyAddress(r *http.Request, trusted []*net.IPNet) string {
	direct := clientIP(r)
	isTrusted := func(ip net.IP) bool {
		for _, network := range trusted {
			if network.Contains(ip) {
				return true
			}
		}
		return false
	}
	peer := net.ParseIP(direct)
	if peer == nil || !isTrusted(peer) {
		return direct
	}
	raw := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if raw == "" || len(raw) > 4096 {
		return direct
	}
	hops := strings.Split(raw, ",")
	if len(hops) > 32 {
		return direct
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		// Invalid chains fail closed to the directly observed peer.
		if ip == nil {
			return direct
		}
		if !isTrusted(ip) || i == 0 {
			return ip.String()
		}
	}
	return direct
}

// Audit event, one JSON object per line in <store>.audit.jsonl.
type auditEvent struct {
	At     time.Time `json:"at"`
	Event  string    `json:"event"`           // admin_login, join, node_approve, ...
	IP     string    `json:"ip,omitempty"`    // client address when there is one
	Actor  string    `json:"actor,omitempty"` // node name or "admin"
	Detail string    `json:"detail,omitempty"`
	OK     bool      `json:"ok"`
}

// audit appends one event to the audit log. Write failures go to the
// server log only; auditing must never break the request it records.
func (s *Server) audit(r *http.Request, event, actor, detail string, ok bool) {
	ev := auditEvent{At: time.Now().UTC(), Event: event, Actor: actor, Detail: detail, OK: ok}
	if r != nil {
		ev.IP = s.clientAddress(r)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	if s.auditPath == "" {
		return
	}
	f, err := os.OpenFile(s.auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mnet-control: audit: %v\n", err)
		return
	}
	defer f.Close()
	fmt.Fprintln(f, string(b))
}

// Counters for the metrics endpoint. Atomic-free: all increments
// happen under a small mutex; the rates involved are human-scale.
type metrics struct {
	mu           sync.Mutex
	StartedAt    time.Time
	JoinsOK      int64
	JoinsFailed  int64
	LoginsOK     int64
	LoginsFailed int64
	RateLimited  int64
}

func (m *metrics) bump(field *int64) {
	m.mu.Lock()
	*field++
	m.mu.Unlock()
}
