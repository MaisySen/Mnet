package enrollmentpending

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	source "fintex.au/mnet/internal/enrollmentsource"
)

func callbackReady(t *testing.T) (*Journal, time.Time, SourceConfig) {
	t.Helper()
	j, l, now, _ := prepared(t)
	if err := j.RecordCallback(l.AttemptID, l.State, code(), now); err != nil {
		t.Fatal(err)
	}
	return j, now, SourceConfig{SourceOrigin: j.data.SourceOrigin, PrincipalAuthority: j.data.Authority, Allowed: append([]contract.SourceMapping(nil), j.data.Allowed...)}
}
func sourceLimits() source.Limits { return source.Limits{Timeout: time.Second, MaxConnections: 1} }
func TestExchangeSavedPublishesBeforeSendAndSavesOnlyCheckedData(t *testing.T) {
	j, now, c := callbackReady(t)
	raw := exchange(t, j, now)
	var result contract.SourceExchange
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	var calls int
	j.send = func(ctx context.Context, r record, cfg SourceConfig, _ source.Limits, _ time.Time) (contract.SourceExchange, error) {
		calls++
		disk, err := os.ReadFile(j.path)
		if err != nil {
			t.Fatal(err)
		}
		var saved record
		if !strictRecord(disk, &saved) || saved.Phase != "exchange-may-have-started" {
			t.Fatal("private request preceded durable remote uncertainty")
		}
		if r.Credential != saved.Credential || r.Verifier != saved.Verifier || r.Code != saved.Code || cfg.SourceOrigin != saved.SourceOrigin {
			t.Fatal("sent material not from exact saved own intent")
		}
		if err := j.RecordExchange(raw, c.Allowed, now); err == nil {
			t.Fatal("external DATA setter bypassed active request")
		}
		return result, nil
	}
	got, err := j.ExchangeSaved(context.Background(), func() SourceConfig { return c }, func() time.Time { return now }, sourceLimits())
	if err != nil || got != result || calls != 1 {
		t.Fatal("saved exchange failed", err)
	}
	if phase, err := j.Status(); err != nil || phase != "exchanged-data" {
		t.Fatal("result not durably saved", err)
	}
	if _, err := j.ExchangeSaved(context.Background(), func() SourceConfig { return c }, func() time.Time { return now }, sourceLimits()); err == nil || calls != 1 {
		t.Fatal("one-shot exchange automatically repeated")
	}
}
func TestExchangeSavedFailedMarkerMakesZeroPrivateSends(t *testing.T) {
	j, now, c := callbackReady(t)
	calls := 0
	j.send = func(context.Context, record, SourceConfig, source.Limits, time.Time) (contract.SourceExchange, error) {
		calls++
		return contract.SourceExchange{}, nil
	}
	j.fs.rename = func(string, string) error { return injected }
	if _, err := j.ExchangeSaved(context.Background(), func() SourceConfig { return c }, func() time.Time { return now }, sourceLimits()); err == nil {
		t.Fatal("failed marker reported success")
	}
	if calls != 0 || j.data.Phase != "callback-recorded" {
		t.Fatal("private send occurred before durable marker")
	}
}
func TestExchangeSavedRechecksCurrentConfigAndPreservesRemoteUnknown(t *testing.T) {
	for _, kind := range []string{"loss", "source-change", "authority-change", "mapping-change", "backward-clock", "save-failure"} {
		t.Run(kind, func(t *testing.T) {
			j, now, c := callbackReady(t)
			raw := exchange(t, j, now)
			var result contract.SourceExchange
			json.Unmarshal(raw, &result)
			after := false
			j.send = func(context.Context, record, SourceConfig, source.Limits, time.Time) (contract.SourceExchange, error) {
				after = true
				if kind == "loss" {
					return contract.SourceExchange{}, source.ErrUnavailable
				}
				if kind == "save-failure" {
					j.fs.rename = func(string, string) error { return injected }
				}
				return result, nil
			}
			current := func() SourceConfig {
				out := copyConfig(c)
				if after {
					switch kind {
					case "source-change":
						out.SourceOrigin = "https://changed.test"
					case "authority-change":
						out.PrincipalAuthority = "https://changed.test"
					case "mapping-change":
						out.Allowed = nil
					}
				}
				return out
			}
			clock := func() time.Time {
				if after && kind == "backward-clock" {
					return now.Add(-time.Second)
				}
				return now
			}
			got, err := j.ExchangeSaved(context.Background(), current, clock, sourceLimits())
			if !errors.Is(err, ErrRemoteUnknown) || got != (contract.SourceExchange{}) {
				t.Fatal("post-send failure not remote-unknown", err)
			}
			if j.data.Phase != "exchange-may-have-started" {
				t.Fatal("post-send failure lost original remote uncertainty")
			}
		})
	}
}
func TestExchangeSavedCloseCancelsAdmittedRequestAndKeepsMarker(t *testing.T) {
	j, now, c := callbackReady(t)
	dir := j.owner.Parent()
	admitted := make(chan struct{})
	done := make(chan error, 1)
	var calls atomic.Int64
	j.send = func(ctx context.Context, _ record, _ SourceConfig, _ source.Limits, _ time.Time) (contract.SourceExchange, error) {
		calls.Add(1)
		close(admitted)
		<-ctx.Done()
		return contract.SourceExchange{}, ctx.Err()
	}
	go func() {
		_, err := j.ExchangeSaved(context.Background(), func() SourceConfig { return c }, func() time.Time { return now }, sourceLimits())
		done <- err
	}()
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("request not admitted")
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) || !errors.Is(err, ErrRemoteUnknown) {
			t.Fatal("Close claimed remote cancellation was absence", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel admitted request")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	phase, err := reopened.Status()
	if err != nil || phase != "exchange-may-have-started" || calls.Load() != 1 {
		t.Fatal("Close lost durable uncertainty", err)
	}
}

func TestExchangeSavedDefaultClientThroughActualTLS(t *testing.T) {
	// This package otherwise uses only per-instance sender seams. Configure the
	// process system-root input with the disposable test certificate, not a client
	// TLS bypass or an exported custom RoundTripper.
	var wire atomic.Value
	var journal *Journal
	var calls atomic.Int64
	type fixtureWire struct {
		raw            []byte
		code, verifier string
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := wire.Load().(fixtureWire)
		calls.Add(1)
		if phase, err := journal.Status(); err != nil || phase != "exchange-may-have-started" {
			t.Error("TLS request preceded persisted marker")
		}
		if r.URL.Path != "/api/mnet-enrollment-v1/exchange-v2" || r.Method != "POST" {
			t.Error("wrong endpoint")
		}
		var body struct {
			Code     string `json:"code"`
			Verifier string `json:"verifier"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code != expected.code || body.Verifier != expected.verifier {
			t.Error("request did not use exact saved own proof")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(expected.raw)
	}))
	defer server.Close()
	root := filepath.Join(t.TempDir(), "test-roots.pem")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", root)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	j, err := Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	journal = j
	now := time.Now()
	mapping := contract.SourceMapping{WorkspaceID: "org", Issuer: "https://issuer.test", RegistrarOrigin: "https://registrar.test", Audience: "fixture", MappingRevision: "r1", NetworkID: "net"}
	l, err := j.Begin(code(), "linux", "http://127.0.0.1:34567/mnet/enrollment/callback", server.URL, "https://multies.test", []contract.SourceMapping{mapping}, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := j.RecordCallback(l.AttemptID, l.State, code(), now); err != nil {
		t.Fatal(err)
	}
	wire.Store(fixtureWire{exchange(t, j, now), j.data.Code, j.data.Verifier})
	c := SourceConfig{SourceOrigin: server.URL, PrincipalAuthority: j.data.Authority, Allowed: j.data.Allowed}
	if _, err := j.ExchangeSaved(context.Background(), func() SourceConfig { return c }, time.Now, sourceLimits()); err != nil {
		t.Fatal("journal/default-client/TLS integration failed", err)
	}
	if phase, err := j.Status(); err != nil || phase != "exchanged-data" || calls.Load() != 1 {
		t.Fatal("TLS result not saved as DATA", err)
	}
}

func TestExchangeSavedExpiryDuringPublicationKeepsHistoricalDataNotCurrentSuccess(t *testing.T) {
	j, now, c := callbackReady(t)
	raw := exchange(t, j, now)
	var result contract.SourceExchange
	json.Unmarshal(raw, &result)
	j.send = func(context.Context, record, SourceConfig, source.Limits, time.Time) (contract.SourceExchange, error) {
		return result, nil
	}
	reads := 0
	clock := func() time.Time {
		reads++
		if reads >= 3 {
			return now.Add(5 * time.Minute)
		}
		return now
	}
	got, err := j.ExchangeSaved(context.Background(), func() SourceConfig { return c }, clock, sourceLimits())
	if !errors.Is(err, ErrNoLongerCurrent) || errors.Is(err, ErrRemoteUnknown) || got != (contract.SourceExchange{}) {
		t.Fatal("saved stale result mislabeled current or remotely unknown", err)
	}
	if phase, err := j.Status(); err != nil || phase != "exchanged-data" {
		t.Fatal("durable historical response was rolled back", err)
	}
}
