package control

import (
	"bytes"
	"crypto/tls"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"fintex.au/mnet/internal/netmap"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func ingressLimits() registrarIngressLimits {
	return registrarIngressLimits{MaxRequests: 16, MaxConcurrent: 1, MaxBodyBytes: 512, MaxStateBytes: 1 << 20, MaxEntries: 8, Timeout: 100 * time.Millisecond}
}
func ingressServer(t *testing.T, l registrarIngressLimits, next http.Handler) (*Store, *registrarIngress, *httptest.Server) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.CreateNetwork("ingress-fixture"); err != nil {
		t.Fatal(err)
	}
	g, err := s.newRegistrarDevelopmentIngress(l, next)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(g)
	t.Cleanup(server.Close)
	return s, g, server
}
func ingressRequest(t *testing.T, server *httptest.Server, body io.Reader) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodPost, server.URL+registrar.HistoryPath, body)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", registrar.HistoryScheme+registrarContractDigest(29))
	r.Header.Set("Content-Type", "application/json")
	return r
}
func TestDevelopmentIngressCheapRefusalsNeverReachStoreMutex(t *testing.T) {
	modes := []string{"method", "path", "query", "escaped-path", "duplicate-auth", "wrong-purpose", "bad-secret", "large-auth", "empty-cookie", "missing-type", "duplicate-type", "encoding", "cookie", "origin", "oversize", "empty"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			s, _, server := ingressServer(t, ingressLimits(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
			body := "{}"
			if mode == "oversize" {
				body = strings.Repeat("x", 513)
			}
			if mode == "empty" {
				body = ""
			}
			req := ingressRequest(t, server, strings.NewReader(body))
			switch mode {
			case "method":
				req.Method = "GET"
			case "path":
				req.URL.Path = "/other"
			case "query":
				req.URL.RawQuery = "ignored=1"
			case "escaped-path":
				req.URL.RawPath = "/api/%6dnet-enrollment-history-experimental-v1/receipt"
			case "duplicate-auth":
				req.Header.Add("Authorization", req.Header.Get("Authorization"))
			case "wrong-purpose":
				req.Header.Set("Authorization", registrar.WorkflowScheme+registrarContractDigest(29))
			case "large-auth":
				req.Header.Set("Authorization", registrar.HistoryScheme+strings.Repeat("x", 16384))
			case "empty-cookie":
				req.Header.Add("Cookie", "")
			case "bad-secret":
				req.Header.Set("Authorization", registrar.HistoryScheme+"invalid")
			case "missing-type":
				req.Header.Del("Content-Type")
			case "duplicate-type":
				req.Header.Add("Content-Type", "application/json")
			case "encoding":
				req.Header.Set("Content-Encoding", "gzip")
			case "cookie":
				req.Header.Set("Cookie", "ambient=value")
			case "origin":
				req.Header.Set("Origin", "https://ambient.test")
			}
			s.mu.Lock()
			done := make(chan error, 1)
			go func() {
				resp, err := server.Client().Do(req)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode == 200 {
						err = errRegistrarEnrollment
					}
				}
				done <- err
			}()
			select {
			case err := <-done:
				s.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				s.mu.Unlock()
				t.Fatal("cheap invalid request waited on Store mutex")
			}
			if calls.Load() != 0 {
				t.Fatal("cheap refusal reached inner handler")
			}
		})
	}
}
func TestDevelopmentIngressSharedFiniteBudgetAndNoReconstruction(t *testing.T) {
	var calls atomic.Int32
	l := ingressLimits()
	l.MaxRequests = 2
	s, g, server := ingressServer(t, l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	alias := *g
	_ = alias
	for i := 0; i < 4; i++ {
		req := ingressRequest(t, server, strings.NewReader("{}"))
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		expected := 200
		if i >= 2 {
			expected = 429
		}
		if resp.StatusCode != expected {
			t.Fatal("finite request budget", i, resp.StatusCode)
		}
	}
	if calls.Load() != 2 || g.used != 2 || alias.used != 2 {
		t.Fatal("copies did not share exact budget")
	}
	if _, err := s.newRegistrarDevelopmentIngress(l, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); err == nil {
		t.Fatal("reconstruction refunded budget")
	}
}
func TestDevelopmentIngressConcurrencyDoesNotQueueOrSpendRefusal(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	_, g, server := ingressServer(t, ingressLimits(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.WriteHeader(200) }))
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := server.Client().Do(ingressRequest(t, server, strings.NewReader("{}")))
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-entered
	resp, err := server.Client().Do(ingressRequest(t, server, strings.NewReader("{}")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Fatal("concurrency overflow queued", resp.StatusCode)
	}
	close(release)
	<-done
	g.mu.Lock()
	used, active := g.used, g.active
	g.mu.Unlock()
	if used != 1 || active != 0 {
		t.Fatal("capacity refusal changed budget/accounting", used, active)
	}
}
func TestDevelopmentIngressSlowBodyAndStoreClose(t *testing.T) {
	for _, mode := range []string{"slow-body", "closed", "freeze", "count-bound", "file-bound"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			s, g, server := ingressServer(t, ingressLimits(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
			req := ingressRequest(t, server, strings.NewReader("{}"))
			var pipe *io.PipeWriter
			switch mode {
			case "slow-body":
				read, write := io.Pipe()
				pipe = write
				defer write.Close()
				req.Body = read
				req.ContentLength = -1
			case "closed":
				s.Close()
			case "freeze":
				s.mu.Lock()
				s.freezeLocked(ErrStoreUncertain)
				s.mu.Unlock()
			case "count-bound":
				s.mu.Lock()
				for i := 0; i < 9; i++ {
					s.data.Nodes[strings.Repeat("n", i+1)] = &netmap.Node{}
				}
				s.mu.Unlock()
			case "file-bound":
				os.WriteFile(s.ownedPath, bytes.Repeat([]byte{' '}, (1<<20)+1), 0600)
			}
			start := time.Now()
			done := make(chan error, 1)
			go func() {
				resp, err := server.Client().Do(req)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode == 200 {
						err = errRegistrarEnrollment
					}
				}
				done <- err
			}()
			if pipe != nil {
				go func() { _, _ = pipe.Write([]byte("{")) }()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("bounded refusal hung")
			}
			if time.Since(start) > time.Second || calls.Load() != 0 {
				t.Fatal("unsafe/slow ingress reached expensive inner work")
			}
			if mode == "closed" || mode == "freeze" {
				g.mu.Lock()
				closed := g.closed
				g.mu.Unlock()
				if !closed {
					t.Fatal("actual Store lifecycle did not retire ingress")
				}
			}
		})
	}
}
func TestDevelopmentIngressCloseCancelsInnerRequest(t *testing.T) {
	entered := make(chan struct{})
	done := make(chan struct{})
	s, _, server := ingressServer(t, ingressLimits(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(done) }))
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		resp, err := server.Client().Do(ingressRequest(t, server, strings.NewReader("{}")))
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-entered
	s.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel admitted inner work")
	}
	<-finished
}
func TestDevelopmentIngressPlainAndUnsupportedDeadlineRefuse(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateNetwork("ingress-fixture"); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	g, err := s.newRegistrarDevelopmentIngress(ingressLimits(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	if err != nil {
		t.Fatal(err)
	}
	for _, tlsPresent := range []bool{false, true} {
		r := httptest.NewRequest("POST", registrar.HistoryPath, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", registrar.HistoryScheme+registrarContractDigest(29))
		if tlsPresent {
			r.TLS = &tls.ConnectionState{}
		} else {
			r.TLS = nil
		}
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code == 200 || calls.Load() != 0 {
			t.Fatal("unbounded unsupported writer accepted")
		}
	}
}

func TestDevelopmentIngressPublicationCapacityKeepsWholeStore(t *testing.T) {
	for _, mode := range []string{"entry-count", "state-bytes"} {
		t.Run(mode, func(t *testing.T) {
			l := ingressLimits()
			if mode == "entry-count" {
				l.MaxEntries = 1
			}
			s, g, _ := ingressServer(t, l, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			before, _ := os.ReadFile(s.ownedPath)
			version := s.Version()
			if mode == "state-bytes" {
				g.limits.MaxStateBytes = len(before) + 5
			}
			if _, err := cleanName("another-network"); err != nil {
				t.Fatal("capacity candidate name invalid", err)
			}
			if _, err := s.CreateNetwork("another-network"); err != errRegistrarEnrollment {
				t.Fatal("expected exact capacity refusal, got", err)
			}
			after, _ := os.ReadFile(s.ownedPath)
			if !bytes.Equal(before, after) || len(s.data.Networks) != 1 || s.Version() != version || s.Health() != nil {
				t.Fatal("capacity refusal damaged whole Store or notified")
			}
		})
	}
}

func TestDevelopmentIngressBusyStoreRefusesWithoutQueue(t *testing.T) {
	var calls atomic.Int32
	s, _, server := ingressServer(t, ingressLimits(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	s.mu.Lock()
	done := make(chan int, 1)
	go func() {
		resp, err := server.Client().Do(ingressRequest(t, server, strings.NewReader("{}")))
		if err != nil {
			done <- 0
			return
		}
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	select {
	case status := <-done:
		s.mu.Unlock()
		if status != 503 || calls.Load() != 0 {
			t.Fatal("busy owner admission", status)
		}
	case <-time.After(time.Second):
		s.mu.Unlock()
		t.Fatal("valid ingress queued on busy Store")
	}
}

func TestDevelopmentIngressNestedCandidateAndRawStringBound(t *testing.T) {
	for _, mode := range []string{"nested-list", "large-string", "large-registrar-scope"} {
		t.Run(mode, func(t *testing.T) {
			s, g, _ := ingressServer(t, ingressLimits(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			s.mu.Lock()
			defer s.mu.Unlock()
			if mode == "nested-list" {
				for _, network := range s.data.Networks {
					network.ACL = &netmap.ACLPolicy{Rules: make([]netmap.ACLRule, 9)}
				}
			}
			if mode == "large-string" {
				for _, network := range s.data.Networks {
					network.Name = strings.Repeat("x", g.limits.MaxStateBytes+1)
				}
			}
			if mode == "large-registrar-scope" {
				s.data.Registrar = &registrarState{Records: map[string]*registrarRecord{"test": {Scope: registrarScope{Issuer: strings.Repeat("x", g.limits.MaxStateBytes+1)}}}}
			}
			if s.registrarIngressBoundLocked(g.limits) == nil {
				t.Fatal("unbounded nested candidate passed preflight")
			}
		})
	}
}
func TestDevelopmentIngressDeadlineCancelsInnerRequest(t *testing.T) {
	done := make(chan struct{})
	_, _, server := ingressServer(t, ingressLimits(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done(); close(done) }))
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		resp, err := server.Client().Do(ingressRequest(t, server, strings.NewReader("{}")))
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inner request context did not expire")
	}
	<-finished
}
