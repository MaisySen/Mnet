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
	mu  sync.Mutex
	m   map[string]*ipBucket
	r   rate.Limit
	b   int
	now func() time.Time
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
	for ip, bkt := range l.m {
		if now.Sub(bkt.last) > time.Hour {
			delete(l.m, ip)
		}
	}
	bkt, ok := l.m[ip]
	if !ok {
		bkt = &ipBucket{lim: rate.NewLimiter(l.r, l.b)}
		l.m[ip] = bkt
	}
	bkt.last = now
	return bkt.lim.AllowN(now, 1)
}

// limit wraps h with a 429 when the client IP exceeds the bucket.
func (l *ipLimiters) limit(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeErr(w, http.StatusTooManyRequests, "rate limited; retry later")
			return
		}
		h(w, r)
	}
}

// clientIP prefers the first X-Forwarded-For hop (Render and other
// proxies set it), falling back to the direct peer.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
		ev.IP = clientIP(r)
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
