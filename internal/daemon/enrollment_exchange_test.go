package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	source "fintex.au/mnet/internal/enrollmentsource"
)

func TestNativeOwnedExchangeThroughActualTLS(t *testing.T) {
	var handler atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.Load().(http.HandlerFunc)(w, r) }))
	defer server.Close()
	root := filepath.Join(t.TempDir(), "roots.pem")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", root)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	for _, kind := range []string{"success", "policy-before", "state-before", "policy-inflight", "state-inflight", "lost-response", "close-inflight"} {
		t.Run(kind, func(t *testing.T) {
			d, path, rawPolicy := enrollmentContextFixture(t)
			var policy config.LocalConfigV2
			if err := json.Unmarshal(rawPolicy, &policy); err != nil {
				t.Fatal(err)
			}
			policy.SourceOrigin = server.URL
			rawPolicy, _ = json.Marshal(policy)
			if err := os.WriteFile(path, rawPolicy, 0600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(d.stateOwner.path)
			launch, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if status := invokeNativeCallback(t, launch); status != 200 {
				t.Fatal("callback failed", status)
			}
			// Inspect test-owned private file to assert exact already-saved own proof on
			// the wire. No production getter or transport override is introduced.
			var saved struct {
				Code, Verifier string
				Launch         contract.SourceLaunch
				Allowed        []contract.SourceMapping
			}
			journalPath := filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending", "enrollment-pending-v1.json")
			raw, _ := os.ReadFile(journalPath)
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			handler.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != "/api/mnet-enrollment-v1/exchange-v2" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Origin") != "" {
					t.Error("unexpected source transport")
				}
				var proof struct {
					contract.SourceLaunch
					Code     string `json:"code"`
					Verifier string `json:"verifier"`
				}
				if err := json.NewDecoder(r.Body).Decode(&proof); err != nil {
					t.Error(err)
				}
				sum := sha256.Sum256([]byte(proof.Verifier))
				if proof.Code != saved.Code || proof.Verifier != saved.Verifier || proof.SourceLaunch != launch || base64.RawURLEncoding.EncodeToString(sum[:]) != launch.Challenge {
					t.Error("source request not exact saved own intent")
				}
				phase, _ := d.enrollmentPreparation.journal.Status()
				if phase != "exchange-may-have-started" {
					t.Error("source I/O preceded durable marker")
				}
				switch kind {
				case "policy-inflight":
					os.WriteFile(path, append(rawPolicy, ' '), 0600)
				case "state-inflight":
					d.mu.Lock()
					d.state.NodeName = "inflight"
					d.mu.Unlock()
				case "lost-response":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
					} else {
						conn.Close()
					}
					return
				case "close-inflight":
					d.Close()
					return
				}
				token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
				response := contract.SourceExchange{ResponseVersion: 2, Provenance: contract.SourceProvenance{PrincipalAuthority: policy.PrincipalAuthority, PrincipalNamespace: policy.PrincipalNamespace, PrincipalID: "local-user", SessionBinding: token}, Launch: launch, Ticket: token, ID: token, ExpiresAt: time.Now().Add(30 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"), Mapping: saved.Allowed[0], Purpose: "mnet-enrollment"}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(response)
			}))
			if kind == "policy-before" {
				os.WriteFile(path, append(rawPolicy, ' '), 0600)
			}
			if kind == "state-before" {
				d.mu.Lock()
				d.state.NodeName = "before"
				d.mu.Unlock()
			}
			result, err := d.exchangeNativeEnrollment(context.Background(), source.Limits{Timeout: time.Second, MaxConnections: 1})
			switch kind {
			case "success":
				if err != nil || result.Launch != launch || result.Provenance.PrincipalID != "local-user" || calls.Load() != 1 {
					t.Fatal("owned exchange failed", err, calls.Load())
				}
				phase, _ := d.enrollmentPreparation.journal.Status()
				if phase != "exchanged-data" {
					t.Fatal("DATA not durable")
				}
			case "policy-before", "state-before":
				if err == nil || calls.Load() != 0 || result != (contract.SourceExchange{}) {
					t.Fatal("changed context sent own proof", err, calls.Load())
				}
			default:
				if !errors.Is(err, pending.ErrRemoteUnknown) || result != (contract.SourceExchange{}) || calls.Load() != 1 {
					t.Fatal("inflight loss/change not unknown", err, calls.Load())
				}
			}
			after, _ := os.ReadFile(d.stateOwner.path)
			if !bytes.Equal(before, after) || d.state.NodeID != "" || d.state.NodeToken != "" || d.cc != nil || d.dev != nil {
				t.Fatal("source DATA installed identity/client/device")
			}
			if kind != "policy-before" && kind != "state-before" {
				if _, err := d.exchangeNativeEnrollment(context.Background(), source.Limits{Timeout: time.Second, MaxConnections: 1}); err == nil || calls.Load() != 1 {
					t.Fatal("automatic one-shot retry", err, calls.Load())
				}
			}
		})
	}
}
