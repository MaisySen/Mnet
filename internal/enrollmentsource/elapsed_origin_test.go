package enrollmentsource

import (
	"context"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Delay legitimate local admission before HTTP without changing the supplied
// observation clock or returning a deliberately future-dated source response.
type delayedAdmissionContext struct {
	context.Context
	first   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (c *delayedAdmissionContext) Err() error {
	if c.first.CompareAndSwap(false, true) {
		close(c.entered)
		<-c.release
	}
	return c.Context.Err()
}
func TestReserveElapsedIncludesLocalAdmissionBeforeHTTP(t *testing.T) {
	for _, mode := range []string{"service", "pool"} {
		t.Run(mode, func(t *testing.T) {
			request, response, _ := serviceFixture(t)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				response.CheckedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			service, _ := testService(t, server, request, response)
			pool := setupPool(t, []PoolSource{poolConfig(t, server, "a")}, []*httptest.Server{server}, poolLimits())
			ctx := &delayedAdmissionContext{Context: context.Background(), entered: make(chan struct{}), release: make(chan struct{})}
			done := make(chan error, 1)
			now := time.Now()
			go func() {
				var err error
				if mode == "service" {
					_, err = service.Reserve(ctx, request, now)
				} else {
					_, err = pool.Reserve(ctx, "a", request, now)
				}
				done <- err
			}()
			<-ctx.entered
			time.Sleep(50 * time.Millisecond)
			close(ctx.release)
			if err := <-done; err != nil {
				t.Fatal("valid observation refused after local pre-send delay", err)
			}
			if calls.Load() != 1 {
				t.Fatal("unexpected request count")
			}
		})
	}
}

// Delay specifically under the pool lock, after its initial context check, to
// catch propagation that fixes Service but still drops Pool admission time.
func TestReserveElapsedIncludesPoolMutexWait(t *testing.T) {
	request, response, _ := serviceFixture(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.CheckedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	p := setupPool(t, []PoolSource{poolConfig(t, server, "a")}, []*httptest.Server{server}, poolLimits())
	p.mu.Lock()
	entered := make(chan struct{})
	done := make(chan error, 1)
	now := time.Now()
	go func() { close(entered); _, err := p.Reserve(context.Background(), "a", request, now); done <- err }()
	<-entered
	time.Sleep(50 * time.Millisecond)
	p.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal("valid observation refused after pool mutex wait", err)
	}
}

// Parsing still refuses genuinely future source evidence. Elapsed correction
// must not widen clock tolerance, turn off timing checks or invent freshness.
func TestReserveElapsedStillRefusesActualFutureObservation(t *testing.T) {
	request, response, _ := serviceFixture(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.CheckedAt = time.Now().Add(time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
		response.Deadline = time.Now().Add(2 * time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	service, _ := testService(t, server, request, response)
	if got, err := service.Reserve(context.Background(), request, time.Now()); err == nil || got != (contract.SourceReserve{}) {
		t.Fatal("actual future source accepted")
	}
}
