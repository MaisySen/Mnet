package enrollmentsource

import (
	"bytes"
	"context"
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

func serviceFixture(t *testing.T) (contract.SourceReserveRequest, contract.SourceReserve, time.Time) {
	t.Helper()
	raw, err := os.ReadFile("../enrollmentcontract/testdata/multies-provenance-source-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Exchange contract.SourceExchange `json:"exchange"`
		Reserve  contract.SourceReserve  `json:"reserve"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	f.Reserve.CheckedAt = now.Add(-time.Millisecond).UTC().Format("2006-01-02T15:04:05.000Z")
	f.Reserve.Deadline = now.Add(time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
	return contract.SourceReserveRequest{Ticket: f.Exchange.Ticket, HandoffID: f.Exchange.ID, ReserveID: f.Reserve.ReserveID, Launch: f.Exchange.Launch, Mapping: f.Exchange.Mapping}, f.Reserve, now
}
func testService(t *testing.T, server *httptest.Server, request contract.SourceReserveRequest, response contract.SourceReserve) (*Service, string) {
	t.Helper()
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{17}, 32))
	m := request.Mapping
	s, err := NewService(ServiceScope{SourceOrigin: server.URL, PrincipalAuthority: response.Provenance.PrincipalAuthority, KeyID: "fixture-key", ServiceID: response.ServiceID, Issuer: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience}, secret, []contract.SourceMapping{m}, Limits{Timeout: time.Second, MaxConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	s.transport.TLSClientConfig.RootCAs = roots
	t.Cleanup(s.CloseIdleConnections)
	return s, secret
}
func TestServiceReserveExactWireScopeAndActualDescriptorDATA(t *testing.T) {
	request, response, now := serviceFixture(t)
	var expectedAuthorization string
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.ProtoMajor != 1 || r.URL.Path != "/api/mnet-enrollment-service-v1/reserve-evidence-v2" || r.URL.RawQuery != "" {
			t.Error("wrong fixed service endpoint")
		}
		if r.Header.Get("Authorization") != expectedAuthorization || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.Header.Get("Referer") != "" {
			t.Error("service authentication or ambient disclosure mismatch")
		}
		var got contract.SourceReserveRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got != request {
			t.Error("reserve request changed")
		}
		descriptor, err := contract.ServiceReserveDescriptorDigest(got, response.ServiceID)
		if err != nil || descriptor != response.DescriptorDigest {
			t.Error("actual descriptor identity changed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Set-Cookie", "ambient=not-for-service")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	s, secret := testService(t, server, request, response)
	expectedAuthorization = "Bearer fixture-key." + secret
	for i := 0; i < 2; i++ {
		got, err := s.Reserve(context.Background(), request, now)
		if err != nil || got != response {
			t.Fatal("service source DATA refused", err)
		}
	}
	if calls.Load() != 2 || s.http.Jar != nil || s.transport.Proxy != nil || !s.transport.DisableKeepAlives || s.transport.ForceAttemptHTTP2 {
		t.Fatal("service transport authority/retry settings changed")
	}
	for _, text := range []string{fmt.Sprint(s), fmt.Sprintf("%#v", s), fmt.Sprint(*s), fmt.Sprint(request), fmt.Sprint(response)} {
		if strings.Contains(text, secret) || strings.Contains(text, request.Ticket) {
			t.Fatal("service formatter exposed private transport material")
		}
	}
}
func TestServiceRedirectNeverDisclosesServiceSecretOrTicket(t *testing.T) {
	request, response, now := serviceFixture(t)
	var targets atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targets.Add(1) }))
	defer target.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
			}))
			defer server.Close()
			s, _ := testService(t, server, request, response)
			if _, err := s.Reserve(context.Background(), request, now); !errors.Is(err, ErrUnavailable) {
				t.Fatal("service redirect accepted", err)
			}
		})
	}
	if targets.Load() != 0 {
		t.Fatal("private service request followed redirect")
	}
}
func TestServiceRefusesUnconfiguredRequestBeforePrivateSend(t *testing.T) {
	request, response, now := serviceFixture(t)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	s, _ := testService(t, server, request, response)
	changed := request
	changed.Mapping.NetworkID = "different-network"
	if _, err := s.Reserve(context.Background(), changed, now); !errors.Is(err, ErrInvalid) {
		t.Fatal("unconfigured mapping sent", err)
	}
	changed = request
	changed.Ticket = "bad"
	if _, err := s.Reserve(context.Background(), changed, now); !errors.Is(err, ErrInvalid) {
		t.Fatal("bad private ticket sent", err)
	}
	if calls.Load() != 0 {
		t.Fatal("private request sent before strict scope validation")
	}
}
func TestServiceResponseLossAfterReadDoesNotRetry(t *testing.T) {
	request, response, now := serviceFixture(t)
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
	s, _ := testService(t, server, request, response)
	if got, err := s.Reserve(context.Background(), request, now); !errors.Is(err, ErrUnavailable) || got != (contract.SourceReserve{}) || calls.Load() != 1 {
		t.Fatal("lost immutable reserve response retried or accepted", err)
	}
}
func TestServiceResponseCannotChooseAuthorityScopeOrGrant(t *testing.T) {
	request, response, now := serviceFixture(t)
	for _, kind := range []string{"authority", "service", "descriptor", "mapping", "grant", "historical", "error"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				changed := response
				switch kind {
				case "authority":
					changed.Provenance.PrincipalAuthority = "https://unconfigured.test"
				case "service":
					changed.ServiceID = "unconfigured-service"
				case "descriptor":
					changed.DescriptorDigest = request.Launch.AttemptID
				case "mapping":
					changed.Mapping.NetworkID = "unconfigured-network"
				case "grant":
					changed.AuthorityGranted = true
				case "historical":
					changed.Status = "consumed"
				case "error":
					w.WriteHeader(403)
					io.WriteString(w, request.Ticket)
					return
				}
				json.NewEncoder(w).Encode(changed)
			}))
			defer server.Close()
			s, _ := testService(t, server, request, response)
			if got, err := s.Reserve(context.Background(), request, now); !errors.Is(err, ErrUnavailable) || got != (contract.SourceReserve{}) || strings.Contains(fmt.Sprint(err), request.Ticket) {
				t.Fatal("unconfigured/granted/error service response accepted or echoed", err)
			}
		})
	}
}

func TestServiceConstructorPinsDefensiveExactScope(t *testing.T) {
	request, response, _ := serviceFixture(t)
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{17}, 32))
	m := request.Mapping
	scope := ServiceScope{SourceOrigin: "https://source.test", PrincipalAuthority: response.Provenance.PrincipalAuthority, KeyID: "fixture-key", ServiceID: response.ServiceID, Issuer: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience}
	allowed := []contract.SourceMapping{m}
	client, err := NewService(scope, secret, allowed, Limits{Timeout: time.Second, MaxConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	allowed[0].NetworkID = "mutated-after-construction"
	if client.allowed[0] != m {
		t.Fatal("source client borrowed mutable config list")
	}
	for _, kind := range []string{"source", "authority", "key", "service", "issuer", "registrar", "audience", "secret", "duplicate"} {
		c := scope
		list := []contract.SourceMapping{m}
		candidate := secret
		switch kind {
		case "source":
			c.SourceOrigin = "http://localhost:1234"
		case "authority":
			c.PrincipalAuthority = "https://authority.test/path"
		case "key":
			c.KeyID = "key.with.dot"
		case "service":
			c.ServiceID = "service with space"
		case "issuer":
			c.Issuer = "https://different-issuer.test"
		case "registrar":
			c.RegistrarOrigin = "https://different-registrar.test"
		case "audience":
			c.Audience = "different-audience"
		case "secret":
			candidate = "not-secret"
		case "duplicate":
			list = append(list, m)
		}
		if made, err := NewService(c, candidate, list, Limits{Timeout: time.Second, MaxConnections: 1}); err == nil {
			made.CloseIdleConnections()
			t.Fatal("invalid service config accepted", kind)
		}
	}
}
func TestServiceUnknownTLSCannotReceiveBearer(t *testing.T) {
	request, response, now := serviceFixture(t)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	trusted, secret := testService(t, server, request, response)
	client, err := NewService(trusted.scope, secret, []contract.SourceMapping{request.Mapping}, Limits{Timeout: time.Second, MaxConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if _, err := client.Reserve(context.Background(), request, now); !errors.Is(err, ErrUnavailable) {
		t.Fatal("untrusted TLS accepted service auth", err)
	}
	if calls.Load() != 0 {
		t.Fatal("private service material reached untrusted TLS HTTP endpoint")
	}
}
