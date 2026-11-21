package enrollmentregistrar

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func workflowFixture(t *testing.T, origin string) (contract.SourceExchange, contract.Challenge, string, WorkflowLimits) {
	t.Helper()
	raw, err := os.ReadFile("../enrollmentcontract/testdata/multies-provenance-source-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Exchange contract.SourceExchange `json:"exchange"`
		Reserve  contract.SourceReserve  `json:"reserve"`
	}
	if json.Unmarshal(raw, &data) != nil {
		t.Fatal("fixture")
	}
	original, secret := fixture(t, origin)
	own := data.Exchange
	own.Mapping.RegistrarOrigin = origin
	own.Launch.CredentialDigest, _ = contract.NativeCredentialSecretDigest(secret)
	own.ExpiresAt = time.Now().Add(time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
	p, m := own.Provenance, own.Mapping
	now := time.Now().UnixMilli()
	descriptor, _ := contract.SourceDescriptorDigest(own, data.Reserve.ServiceID, data.Reserve.ReserveID)
	c := original
	c.Binding = contract.Binding{PrincipalAuthority: p.PrincipalAuthority, PrincipalNamespace: p.PrincipalNamespace, PrincipalID: p.PrincipalID, SessionBinding: p.SessionBinding, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, ServiceID: data.Reserve.ServiceID, WorkspaceID: m.WorkspaceID, NetworkID: m.NetworkID, MappingRevision: m.MappingRevision, HandoffID: own.ID, AttemptID: own.Launch.AttemptID, ReserveID: data.Reserve.ReserveID, DescriptorDigest: descriptor, DescriptorEncodingVersion: 1, DevicePublicKey: own.Launch.DevicePublicKey, CredentialDigest: own.Launch.CredentialDigest, CheckedAtMS: now, ReserveDeadlineMS: now + 30000, AccessUntilMS: now + 5000}
	c.ExpiresAtMS = now + 5000
	l := WorkflowLimits{Timeout: time.Second, MaxChallengeLifetime: 10 * time.Second, MaxReserveLifetime: time.Minute, Timing: contract.FixtureTiming{MaxObservationAgeMS: 20000, MaxLeaseMS: 10000, ClockUncertaintyMS: 1}}
	return own, c, secret, l
}
func TestWorkflowTransportExactOriginOneShotAndFailureNoRetry(t *testing.T) {
	var targetCalls atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	for _, mode := range []string{"success", "redirect", "cookie", "cache", "encoding", "wrong-principal", "lost"} {
		t.Run(mode, func(t *testing.T) {
			var own contract.SourceExchange
			var challenge contract.Challenge
			var secret string
			var calls atomic.Int64
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != WorkflowPrefix+"begin" || r.Header.Get("Authorization") != WorkflowScheme+secret || r.URL.RawQuery != "" || r.ProtoMajor != 1 {
					t.Error("wrong exact-origin workflow")
				}
				for _, header := range []string{"Cookie", "Origin", "Referer", "X-Node-ID", "X-Node-Token"} {
					if r.Header.Get(header) != "" {
						t.Error("ambient authority", header)
					}
				}
				raw, _ := io.ReadAll(r.Body)
				input, err := contract.DecodeWorkflowBeginWire(raw, challenge.Binding.ServiceID)
				if err != nil || input.Request.Launch != own.Launch || input.Request.Ticket != own.Ticket {
					t.Error("saved source request changed", err)
				}
				headers(w)
				switch mode {
				case "redirect":
					w.Header().Set("Location", target.URL)
					w.WriteHeader(307)
					return
				case "cookie":
					w.Header().Set("Set-Cookie", "unused=1; Secure")
				case "cache":
					w.Header().Set("Cache-Control", "public")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "wrong-principal":
					changed := challenge
					changed.Binding.PrincipalID += "x"
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: changed})
					return
				case "lost":
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: challenge})
			}))
			defer server.Close()
			var l WorkflowLimits
			own, challenge, secret, l = workflowFixture(t, server.URL)
			c, err := NewWorkflow(own, challenge.Binding.ServiceID, challenge.Binding.ReserveID, secret, l)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			c.transport.TLSClientConfig.RootCAs = roots
			if c.client.Jar != nil || c.transport.Proxy != nil || c.transport.TLSClientConfig.InsecureSkipVerify || c.transport.ForceAttemptHTTP2 || !c.transport.DisableKeepAlives {
				t.Fatal("unsafe ambient transport")
			}
			got, err := c.Begin(context.Background())
			if mode == "success" || mode == "cookie" {
				if err != nil || got != challenge {
					t.Fatal("valid exact owned begin", err)
				}
			} else if err == nil || got != (contract.Challenge{}) {
				t.Fatal("unsafe response accepted")
			}
			if _, err := c.Begin(context.Background()); err == nil || calls.Load() != 1 {
				t.Fatal("request retried or refunded")
			}
		})
	}
	if targetCalls.Load() != 0 {
		t.Fatal("secret followed redirect")
	}
}
