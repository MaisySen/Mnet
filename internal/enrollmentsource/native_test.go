package enrollmentsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func fixture(t *testing.T) (contract.SourceExchange, string, string, time.Time) {
	t.Helper()
	raw, err := os.ReadFile("../enrollmentcontract/testdata/multies-provenance-source-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Exchange contract.SourceExchange `json:"exchange"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	// Deliberately disconnected transport fixture. Producer/parser equivalence is
	// tested separately. No original verifier or user token is captured here.
	verifier := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{11}, 32))
	code := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{12}, 32))
	sum := sha256.Sum256([]byte(verifier))
	fixture.Exchange.Launch.Challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	now := time.Now()
	fixture.Exchange.ExpiresAt = now.Add(time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
	return fixture.Exchange, code, verifier, now
}
func client(t *testing.T, server *httptest.Server, authority string) *Native {
	t.Helper()
	n, err := NewNative(server.URL, authority, Limits{Timeout: time.Second, MaxConnections: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Tests explicitly trust only the disposable TLS server certificate. The
	// production constructor has no caller TLS/root/transport injection interface.
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	n.transport.TLSClientConfig.RootCAs = roots
	t.Cleanup(n.CloseIdleConnections)
	return n
}
func TestHTTPSOwnExchangeExactWireNoAmbientAuthOrCookies(t *testing.T) {
	own, code, verifier, now := fixture(t)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.ProtoMajor != 1 {
			t.Error("one-shot transport negotiated unexpected multiplexed protocol")
		}
		if r.Method != "POST" || r.URL.Path != "/api/mnet-enrollment-v1/exchange-v2" || r.URL.RawQuery != "" {
			t.Error("wrong endpoint")
		}
		for _, key := range []string{"Cookie", "Origin", "Authorization", "Referer"} {
			if r.Header.Get(key) != "" {
				t.Error("ambient authority disclosed")
			}
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Cache-Control") != "no-store" {
			t.Error("private request policy missing")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Error(err)
		}
		if len(payload) != 11 {
			t.Error("unexpected proof fields")
		}
		var gotCode, gotVerifier string
		json.Unmarshal(payload["code"], &gotCode)
		json.Unmarshal(payload["verifier"], &gotVerifier)
		if gotCode != code || gotVerifier != verifier {
			t.Error("private proof not exact")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Set-Cookie", "ambient=not-for-next-request; Secure")
		json.NewEncoder(w).Encode(own)
	}))
	defer server.Close()
	n := client(t, server, own.Provenance.PrincipalAuthority)
	for i := 0; i < 2; i++ {
		got, err := n.Exchange(context.Background(), own.Launch, server.URL, own.Provenance.PrincipalAuthority, code, verifier, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, now)
		if err != nil || got != own {
			t.Fatal("TLS source data mismatch", err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected retry")
	}
	if n.http.Jar != nil || n.transport.Proxy != nil || n.transport.TLSClientConfig.InsecureSkipVerify || !n.transport.DisableKeepAlives || n.transport.ForceAttemptHTTP2 || len(n.transport.TLSNextProto) != 0 || len(n.transport.TLSClientConfig.NextProtos) != 1 || n.transport.TLSClientConfig.NextProtos[0] != "http/1.1" {
		t.Fatal("ambient transport/TLS bypass")
	}
}
func TestRedirectNeverDisclosesOneShotNativeProof(t *testing.T) {
	own, code, verifier, now := fixture(t)
	var sourceCalls, targetCalls atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sourceCalls.Add(1)
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
			}))
			defer server.Close()
			n := client(t, server, own.Provenance.PrincipalAuthority)
			if _, err := n.Exchange(context.Background(), own.Launch, server.URL, own.Provenance.PrincipalAuthority, code, verifier, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, now); !errors.Is(err, ErrUnavailable) {
				t.Fatal("redirect accepted", err)
			}
		})
	}
	if targetCalls.Load() != 0 || sourceCalls.Load() != 5 {
		t.Fatal("redirect followed or request retried")
	}
}
func TestUntrustedTLSCertificateRefused(t *testing.T) {
	own, code, verifier, now := fixture(t)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	n, err := NewNative(server.URL, own.Provenance.PrincipalAuthority, Limits{Timeout: time.Second, MaxConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer n.CloseIdleConnections()
	if _, err := n.Exchange(context.Background(), own.Launch, server.URL, own.Provenance.PrincipalAuthority, code, verifier, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, now); !errors.Is(err, ErrUnavailable) {
		t.Fatal("untrusted TLS accepted", err)
	}
	if calls.Load() != 0 {
		t.Fatal("proof reached untrusted TLS source")
	}
}
func TestMalformedErrorBodiesAndCachePolicyNeverEscape(t *testing.T) {
	own, code, verifier, now := fixture(t)
	for _, kind := range []string{"status", "html", "cache", "encoding", "oversize", "unknown", "future-flags", "duplicate", "wrong-authority"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				switch kind {
				case "status":
					w.WriteHeader(403)
					io.WriteString(w, verifier)
					return
				case "html":
					w.Header().Set("Content-Type", "text/html")
				case "cache":
					w.Header().Set("Cache-Control", "public, max-age=3600")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "oversize":
					io.WriteString(w, strings.Repeat(" ", 65537))
					return
				case "unknown":
					io.WriteString(w, `{"secret":`+fmt.Sprintf("%q", verifier)+`}`)
					return
				case "future-flags":
					other := own
					other.Enrolled = true
					json.NewEncoder(w).Encode(other)
					return
				case "duplicate":
					w.Header().Add("Content-Type", "application/json")
				case "wrong-authority":
					other := own
					other.Provenance.PrincipalAuthority = "https://other.test"
					json.NewEncoder(w).Encode(other)
					return
				}
				json.NewEncoder(w).Encode(own)
			}))
			defer server.Close()
			n := client(t, server, own.Provenance.PrincipalAuthority)
			got, err := n.Exchange(context.Background(), own.Launch, server.URL, own.Provenance.PrincipalAuthority, code, verifier, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, now)
			if !errors.Is(err, ErrUnavailable) || got != (contract.SourceExchange{}) {
				t.Fatal("malformed response accepted", err)
			}
			if strings.Contains(fmt.Sprint(err), verifier) {
				t.Fatal("server error body exposed")
			}
		})
	}
}
func TestPreflightRefusesRemovedConfigAndWrongVerifierBeforeSending(t *testing.T) {
	own, code, verifier, now := fixture(t)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	n := client(t, server, own.Provenance.PrincipalAuthority)
	changed := own.Mapping
	changed.MappingRevision = "removed"
	for _, p := range []struct {
		v       string
		current []contract.SourceMapping
	}{{"wrong", []contract.SourceMapping{own.Mapping}}, {verifier, nil}, {verifier, []contract.SourceMapping{changed}}} {
		if _, err := n.Exchange(context.Background(), own.Launch, server.URL, own.Provenance.PrincipalAuthority, code, p.v, []contract.SourceMapping{own.Mapping}, p.current, now); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid preflight accepted", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("private proof sent after failed preflight")
	}
	for _, origin := range []string{"http://localhost:1234", "https://user@site.test", "https://source.test/", "https://source.test?q=x"} {
		if _, err := NewNative(origin, own.Provenance.PrincipalAuthority, Limits{Timeout: time.Second, MaxConnections: 1}); err == nil {
			t.Fatal("inexact source origin accepted")
		}
	}
}

func TestOriginalSourceAndAuthorityCannotBeReplacedBeforePrivateSend(t *testing.T) {
	own, code, verifier, now := fixture(t)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	n := client(t, server, own.Provenance.PrincipalAuthority)
	for _, p := range []struct{ origin, authority string }{{"https://other-source.test", own.Provenance.PrincipalAuthority}, {server.URL, "https://other-authority.test"}} {
		if _, err := n.Exchange(context.Background(), own.Launch, p.origin, p.authority, code, verifier, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, now); !errors.Is(err, ErrInvalid) {
			t.Fatal("changed original source accepted", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("private proof sent outside original source snapshot")
	}
}
func TestResponseLossIsRemoteOutcomeUnknownAndNoAutomaticRetry(t *testing.T) {
	own, code, verifier, now := fixture(t)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	defer server.Close()
	n := client(t, server, own.Provenance.PrincipalAuthority)
	got, err := n.Exchange(context.Background(), own.Launch, server.URL, own.Provenance.PrincipalAuthority, code, verifier, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, now)
	if !errors.Is(err, ErrUnavailable) || got != (contract.SourceExchange{}) || calls.Load() != 1 {
		t.Fatal("response loss retried or reported safe", err)
	}
}
func TestResponseElapsedTimeConsumesDeadline(t *testing.T) {
	own, code, verifier, now := fixture(t)
	own.ExpiresAt = now.Add(30 * time.Millisecond).UTC().Format("2006-01-02T15:04:05.000Z")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(70 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(own)
	}))
	defer server.Close()
	n := client(t, server, own.Provenance.PrincipalAuthority)
	if _, err := n.Exchange(context.Background(), own.Launch, server.URL, own.Provenance.PrincipalAuthority, code, verifier, []contract.SourceMapping{own.Mapping}, []contract.SourceMapping{own.Mapping}, now); !errors.Is(err, ErrUnavailable) {
		t.Fatal("expired response accepted using request-start time", err)
	}
}
