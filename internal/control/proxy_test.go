package control

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestForwardedHeaderCannotBypassRateLimit(t *testing.T) {
	l := newIPLimiters(60, 1)
	h := l.limit(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	for i, want := range []int{200, 429, 429} {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = "198.51.100.8:5000"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i))
		rec := httptest.NewRecorder()
		h(rec, r)
		if rec.Code != want {
			t.Fatalf("request %d got %d want %d; spoofed header bypassed bucket", i, rec.Code, want)
		}
	}
}

func TestTrustedProxyAddress(t *testing.T) {
	_, private, _ := net.ParseCIDR("10.0.0.0/8")
	_, loopback, _ := net.ParseCIDR("::1/128")
	trusted := []*net.IPNet{private, loopback}
	for _, tc := range []struct{ name, remote, xff, want string }{
		{"untrusted direct", "198.51.100.8:80", "203.0.113.9", "198.51.100.8"},
		{"trusted direct", "10.1.2.3:80", "203.0.113.9", "203.0.113.9"},
		{"forged prefix", "10.1.2.3:80", "192.0.2.1, 198.51.100.8", "198.51.100.8"},
		{"trusted suffix", "10.1.2.3:80", "192.0.2.1, 198.51.100.8, 10.2.3.4", "198.51.100.8"},
		{"malformed suffix", "10.1.2.3:80", "203.0.113.9, invalid", "10.1.2.3"},
		{"empty hop", "10.1.2.3:80", "203.0.113.9, ", "10.1.2.3"},
		{"missing", "10.1.2.3:80", "", "10.1.2.3"},
		{"ipv6", "[::1]:80", "2001:db8::8", "2001:db8::8"},
		{"oversized", "10.1.2.3:80", strings.Repeat("x", 4097), "10.1.2.3"},
		{"too many hops", "10.1.2.3:80", strings.Repeat("10.2.3.4,", 32) + "10.2.3.4", "10.1.2.3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			r.Header.Set("X-Forwarded-For", tc.xff)
			if got := proxyAddress(r, trusted); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:80"
	r.Header.Add("X-Forwarded-For", "192.0.2.1")
	r.Header.Add("X-Forwarded-For", "198.51.100.8, 10.2.3.4")
	if got := proxyAddress(r, trusted); got != "198.51.100.8" {
		t.Fatalf("multi-line headers: %s", got)
	}
}

func TestProxyOptionsRejectInvalidCIDRs(t *testing.T) {
	for _, bad := range []string{"", "not-a-cidr", "10.1.2.3", "10.0.0.0/99"} {
		if _, _, err := NewServerWithOptions(nil, nil, "", "pw", ServerOptions{TrustedProxyCIDRs: []string{bad}}); err == nil {
			t.Fatalf("accepted invalid proxy %q", bad)
		}
	}
}

func TestProxyOptionsApplyToRateLimitAndAudit(t *testing.T) {
	st, err := openTestStore(t, t.TempDir()+"/state.json")
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := NewServerWithOptions(st, nil, "https://example.test", "pw", ServerOptions{TrustedProxyCIDRs: []string{"10.0.0.0/8"}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "10.1.2.3:80"
	r.Header.Set("X-Forwarded-For", "203.0.113.8")
	if s.loginLim.address(r) != "203.0.113.8" || s.joinLim.address(r) != "203.0.113.8" {
		t.Fatal("proxy policy not wired into both limiters")
	}
	s.audit(r, "test", "", "", true)
	data, err := os.ReadFile(s.auditPath)
	if err != nil {
		t.Fatal(err)
	}
	var ev auditEvent
	if err = json.Unmarshal(data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.IP != "203.0.113.8" {
		t.Fatalf("audit address %q", ev.IP)
	}
}

func TestIPBucketCapFailsClosedWithoutReset(t *testing.T) {
	l := newIPLimiters(60, 1)
	now := time.Now()
	l.now = func() time.Time { return now }
	if !l.allow("original") {
		t.Fatal("first request refused")
	}
	for i := 1; i < maxIPBuckets; i++ {
		l.m[fmt.Sprint(i)] = &ipBucket{last: now}
	}
	if l.allow("new-address") {
		t.Fatal("address table exceeded cap")
	}
	if len(l.m) != maxIPBuckets {
		t.Fatalf("table size %d", len(l.m))
	}
	if l.allow("original") {
		t.Fatal("capacity pressure reset existing budget")
	}
	now = now.Add(2 * time.Hour)
	if !l.allow("new-address") || len(l.m) != 1 {
		t.Fatal("expired capacity not reclaimed")
	}
}
