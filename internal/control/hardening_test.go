package control

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIPLimitersThrottle(t *testing.T) {
	l := newIPLimiters(60, 3)
	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("burst %d refused", i)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("fourth immediate request should be refused")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("a different IP must have its own bucket")
	}
}

func TestIPLimitersRefill(t *testing.T) {
	now := time.Now()
	l := newIPLimiters(60, 1)
	l.now = func() time.Time { return now }
	if !l.allow("1.2.3.4") || l.allow("1.2.3.4") {
		t.Fatal("burst of one, then refuse")
	}
	now = now.Add(2 * time.Second) // 60/min refills one token per second
	if !l.allow("1.2.3.4") {
		t.Fatal("bucket should have refilled")
	}
}

func TestIPLimitersExpire(t *testing.T) {
	now := time.Now()
	l := newIPLimiters(60, 1)
	l.now = func() time.Time { return now }
	l.allow("1.2.3.4")
	now = now.Add(2 * time.Hour)
	l.allow("9.9.9.9") // triggers the sweep
	if len(l.m) != 1 {
		t.Fatalf("stale bucket not expired: %d buckets", len(l.m))
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	if got := clientIP(r); got != "10.0.0.1" {
		t.Fatalf("remote addr: %q", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("xff first hop: %q", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	next := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, httptest.NewRequest("GET", "/api/map", nil))
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	if rec.Header().Get("Content-Security-Policy") != "default-src 'none'" {
		t.Fatal("API responses must deny content loading")
	}
	rec = httptest.NewRecorder()
	next.ServeHTTP(rec, httptest.NewRequest("GET", "/admin", nil))
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "" {
		t.Fatalf("dashboard must keep inline scripts, got CSP %q", csp)
	}
}

func TestRateLimitedEndpoint429(t *testing.T) {
	l := newIPLimiters(60, 1)
	h := l.limit(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/x", nil))
	if rec.Code != 200 {
		t.Fatalf("first request: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/x", nil))
	if rec.Code != 429 {
		t.Fatalf("second request should be limited, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 needs Retry-After")
	}
}

func TestAuditWritesJSONLines(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := NewServer(st, nil, "https://example.test", "pw")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/admin/api/login", nil)
	r.RemoteAddr = "198.51.100.7:4444"
	s.audit(r, "admin_login", "admin", "", true)
	s.audit(r, "join", "node-1", "net", false)

	f, err := os.Open(dir + "/state.json.audit.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var events []auditEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev auditEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("audit line not json: %v", err)
		}
		events = append(events, ev)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events", len(events))
	}
	if events[0].Event != "admin_login" || events[0].IP != "198.51.100.7" || !events[0].OK {
		t.Fatalf("bad first event: %+v", events[0])
	}
	if events[1].Event != "join" || events[1].OK {
		t.Fatalf("bad second event: %+v", events[1])
	}
	if !strings.HasSuffix(s.auditPath, ".audit.jsonl") {
		t.Fatalf("audit path: %q", s.auditPath)
	}
}
