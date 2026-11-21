package enrollmentsource

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func poolConfig(t *testing.T, server *httptest.Server, id string) PoolSource {
	t.Helper()
	request, response, _ := serviceFixture(t)
	service, secret := testService(t, server, request, response)
	return PoolSource{ID: id, Scope: service.scope, Secret: secret, Allowed: append([]contract.SourceMapping(nil), service.allowed...), Transport: Limits{Timeout: time.Second, MaxConnections: 2}}
}
func setupPool(t *testing.T, entries []PoolSource, servers []*httptest.Server, limits PoolLimits) *Pool {
	t.Helper()
	p, err := NewPool(entries, limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	// Test-only root pin, not a custom production transport or TLS bypass.
	for i, entry := range entries {
		roots := x509.NewCertPool()
		roots.AddCert(servers[i].Certificate())
		p.sources[entry.ID].client.transport.TLSClientConfig.RootCAs = roots
	}
	return p
}
func poolLimits() PoolLimits {
	return PoolLimits{MaxSources: 2, MaxConcurrent: 2, MaxPerSource: 1, MaxAttempts: 10}
}
func newAttempt(request contract.SourceReserveRequest, b byte) contract.SourceReserveRequest {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = b
	}
	request.Launch.AttemptID = encodeTest(raw)
	return request
}
func encodeTest(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }
func TestPoolReservesGlobalAndSourceConcurrencyBeforePrivateIO(t *testing.T) {
	var active, peak, calls atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	handler := func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		n := active.Add(1)
		for {
			old := peak.Load()
			if old >= n || peak.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		w.WriteHeader(503)
	}
	a := httptest.NewTLSServer(http.HandlerFunc(handler))
	defer a.Close()
	b := httptest.NewTLSServer(http.HandlerFunc(handler))
	defer b.Close()
	p := setupPool(t, []PoolSource{poolConfig(t, a, "a"), poolConfig(t, b, "b")}, []*httptest.Server{a, b}, poolLimits())
	request, _, now := serviceFixture(t)
	done := make(chan error, 2)
	go func() { _, err := p.Reserve(context.Background(), "a", request, now); done <- err }()
	<-entered
	if _, err := p.Reserve(context.Background(), "a", newAttempt(request, 7), now); !errors.Is(err, ErrAdmission) || calls.Load() != 1 {
		t.Fatal("per-source refusal performed HTTP", err)
	}
	go func() { _, err := p.Reserve(context.Background(), "b", newAttempt(request, 8), now); done <- err }()
	<-entered
	if _, err := p.Reserve(context.Background(), "b", newAttempt(request, 9), now); !errors.Is(err, ErrAdmission) || calls.Load() != 2 {
		t.Fatal("global refusal performed HTTP", err)
	}
	if peak.Load() != 2 {
		t.Fatal("wrong concurrent count")
	}
	close(release)
	<-done
	<-done
	if p.active != 0 || p.sources["a"].active != 0 || len(p.attempts) != 2 {
		t.Fatal("capacity leak/refund of admitted attempt")
	}
	// The previously capacity-refused attempt wasn't burned. It may be admitted
	// once later, but remote503 makes its final result unknown, never retry-safe.
	if _, err := p.Reserve(context.Background(), "a", newAttempt(request, 7), now); !errors.Is(err, ErrUnavailable) || calls.Load() != 3 {
		t.Fatal("pre-admission refusal burned new attempt", err)
	}
}
func TestPoolOneAttemptForeverEvenChangedDescriptorOrLostResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	defer server.Close()
	limits := poolLimits()
	limits.MaxAttempts = 1
	p := setupPool(t, []PoolSource{poolConfig(t, server, "a")}, []*httptest.Server{server}, limits)
	request, _, now := serviceFixture(t)
	if _, err := p.Reserve(context.Background(), "a", request, now); !errors.Is(err, ErrUnavailable) {
		t.Fatal("lost response not unknown", err)
	}
	changed := request
	changed.ReserveID = newAttempt(request, 5).Launch.AttemptID
	for _, r := range []contract.SourceReserveRequest{request, changed, newAttempt(request, 6)} {
		if _, err := p.Reserve(context.Background(), "a", r, now); !errors.Is(err, ErrAdmission) {
			t.Fatal("one-shot/capacity bypass", err)
		}
	}
	if calls.Load() != 1 || len(p.attempts) != 1 {
		t.Fatal("loss refunded or unbounded history")
	}
}
func TestPoolCloseAndCallerCancelCancelInflightKeepAttemptSpent(t *testing.T) {
	for _, kind := range []string{"close", "caller"} {
		t.Run(kind, func(t *testing.T) {
			entered := make(chan struct{})
			returned := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				close(entered)
				select {
				case <-r.Context().Done():
				case <-time.After(4 * time.Second):
				}
				close(returned)
			}))
			defer server.Close()
			p := setupPool(t, []PoolSource{poolConfig(t, server, "a")}, []*httptest.Server{server}, poolLimits())
			request, _, now := serviceFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := p.Reserve(ctx, "a", request, now); done <- err }()
			<-entered
			if kind == "close" {
				copyHandle := *p
				copyHandle.Close()
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, ErrUnavailable) {
					t.Fatal("cancel not remote unknown", err)
				}
			case <-time.After(time.Second * 2):
				t.Fatal("inflight source not cancelled")
			}
			select {
			case <-returned:
			case <-time.After(time.Second * 2):
				t.Fatal("HTTP cancellation didn't reach source")
			}
			if len(p.attempts) != 1 || p.active != 0 {
				t.Fatal("cancel lost bounded history")
			}
			if _, err := p.Reserve(context.Background(), "a", request, now); err == nil {
				t.Fatal("closed/cancelled request retried")
			}
		})
	}
}
func TestPoolInvalidConfigAndRequestsNoPrivateSend(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	entry := poolConfig(t, server, "a")
	limits := poolLimits()
	limits.MaxAttempts = 0
	if p, err := NewPool([]PoolSource{entry}, limits); err == nil {
		p.Close()
		t.Fatal("zero limits accepted")
	}
	duplicate := entry
	duplicate.ID = "other"
	duplicate.Scope.ServiceID = "other-service"
	if p, err := NewPool([]PoolSource{entry, duplicate}, poolLimits()); err == nil {
		p.Close()
		t.Fatal("alias/service multiplied source budget")
	}
	p := setupPool(t, []PoolSource{entry}, []*httptest.Server{server}, poolLimits())
	entry.Allowed[0].NetworkID = "mutated"
	request, _, now := serviceFixture(t)
	bad := request
	bad.Mapping.NetworkID = "wrong"
	for _, r := range []contract.SourceReserveRequest{{}, bad} {
		if _, err := p.Reserve(context.Background(), "a", r, now); !errors.Is(err, ErrAdmission) {
			t.Fatal("invalid request accepted")
		}
	}
	if _, err := p.Reserve(context.Background(), "unknown", request, now); !errors.Is(err, ErrAdmission) {
		t.Fatal("unknown source admitted")
	}
	if calls.Load() != 0 || len(p.attempts) != 0 {
		t.Fatal("cheap invalid spent private I/O/history")
	}
	if _, err := p.Reserve(context.Background(), "a", request, now); !errors.Is(err, ErrUnavailable) || calls.Load() != 1 {
		t.Fatal("caller alias mutated internal mapping", err)
	}
	for _, text := range []string{fmt.Sprintf("%v %#v", p, *p), fmt.Sprintf("%v %#v", entry, entry)} {
		if strings.Contains(text, entry.Secret) || strings.Contains(text, entry.Scope.SourceOrigin) {
			t.Fatal("pool formatter leaked")
		}
	}
}
func TestPoolSuccessRemainsDATAAndNeverRenewsAttempt(t *testing.T) {
	request, response, now := serviceFixture(t)
	digest, err := contract.ServiceReserveDescriptorDigest(request, response.ServiceID)
	if err != nil {
		t.Fatal(err)
	}
	response.DescriptorDigest = digest
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	p := setupPool(t, []PoolSource{poolConfig(t, server, "a")}, []*httptest.Server{server}, poolLimits())
	got, err := p.Reserve(context.Background(), "a", request, now)
	if err != nil || got != response {
		t.Fatal("strict DATA refused", err)
	}
	if _, err := p.Reserve(context.Background(), "a", request, now); !errors.Is(err, ErrAdmission) || calls.Load() != 1 {
		t.Fatal("success renewed one-shot reserve", err)
	}
}

func TestPoolGlobalCeilingIndependentOfPerSourceCeiling(t *testing.T) {
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { entered <- struct{}{}; <-release; w.WriteHeader(503) })
	a := httptest.NewTLSServer(handler)
	defer a.Close()
	b := httptest.NewTLSServer(handler)
	defer b.Close()
	limits := poolLimits()
	limits.MaxPerSource = 2
	p := setupPool(t, []PoolSource{poolConfig(t, a, "a"), poolConfig(t, b, "b")}, []*httptest.Server{a, b}, limits)
	request, _, now := serviceFixture(t)
	done := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		go func(id string) { _, err := p.Reserve(context.Background(), id, request, now); done <- err }(id)
		select {
		case <-entered:
		case <-time.After(time.Second * 2):
			close(release)
			t.Fatal("source admission timed out")
		}
	}
	_, err := p.Reserve(context.Background(), "a", newAttempt(request, 28), now)
	close(release)
	<-done
	<-done
	if !errors.Is(err, ErrAdmission) {
		t.Fatal("global ceiling bypassed despite spare per-source capacity", err)
	}
}
func TestPoolSameOriginalAttemptBlocksDifferentReserveBeforeHistoryFull(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	p := setupPool(t, []PoolSource{poolConfig(t, server, "a")}, []*httptest.Server{server}, poolLimits())
	request, _, now := serviceFixture(t)
	p.Reserve(context.Background(), "a", request, now)
	changed := request
	changed.ReserveID = newAttempt(request, 37).Launch.AttemptID
	if _, err := p.Reserve(context.Background(), "a", changed, now); !errors.Is(err, ErrAdmission) || calls.Load() != 1 || len(p.attempts) != 1 {
		t.Fatal("same original attempt escaped through changed reserve")
	}
}

func TestPoolRejectsSourceOriginBudgetAliasesWithoutRewritingIdentity(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("config validation performed HTTP") }))
	defer server.Close()
	entry := poolConfig(t, server, "fixture")
	for _, origin := range []string{"https://SOURCE.test", "https://source.test:443", "https://source.test:0444", "https://source.test.", "https://source.test.:444", "https://[0:0:0:0:0:0:0:1]", "https://[::ffff:127.0.0.1]", "https://127.000.0.1", "https://2130706433", "https://é.test"} {
		t.Run(origin, func(t *testing.T) {
			copyEntry := entry
			copyEntry.Scope.SourceOrigin = origin
			p, err := NewPool([]PoolSource{copyEntry}, poolLimits())
			if err == nil {
				p.Close()
				t.Fatal("semantic/config alias accepted")
			}
		})
	}
	for _, origin := range []string{"https://source.test", "https://source.test:444", "https://127.0.0.1:444", "https://[::1]:444", "https://xn--9ca.test"} {
		copyEntry := entry
		copyEntry.Scope.SourceOrigin = origin
		p, err := NewPool([]PoolSource{copyEntry}, poolLimits())
		if err != nil {
			t.Fatal("canonical explicit origin refused", origin, err)
		}
		if p.sources[entry.ID].client.scope.SourceOrigin != origin {
			t.Fatal("exact identity silently rewritten")
		}
		p.Close()
	}
}
