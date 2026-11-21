package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	source "fintex.au/mnet/internal/enrollmentsource"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func nativeWorkflowLimits() registrar.WorkflowLimits {
	return registrar.WorkflowLimits{Timeout: time.Second, MaxChallengeLifetime: 10 * time.Second, MaxReserveLifetime: time.Minute, Timing: contract.FixtureTiming{MaxObservationAgeMS: 20000, MaxLeaseMS: 10000, ClockUncertaintyMS: 1}}
}
func TestNativeWorkflowActualDurableKeyTLSChild(t *testing.T) {
	if os.Getenv("MNET_NATIVE_KEY_WORKFLOW_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestNativeWorkflowActualDurableKeyTLSChild$", "-test.v", "-test.timeout=60s")
		cmd.Env = append(os.Environ(), "MNET_NATIVE_KEY_WORKFLOW_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("native workflow child: %v\n%s", err, out)
		}
		return
	}
	var active atomic.Value
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { active.Load().(http.HandlerFunc)(w, r) }))
	defer s.Close()
	root := filepath.Join(t.TempDir(), "cert.pem")
	os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", root)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	for _, mode := range []string{"success", "wrong-principal", "wrong-key", "wrong-session", "wrong-descriptor", "wrong-confirmation", "policy-before", "state-before", "legacy-before", "legacy-saved", "routes-saved", "policy-after-begin", "state-after-begin", "close-after-begin", "policy-on-submit", "lost-submit"} {
		t.Run(mode, func(t *testing.T) {
			d, policyPath, rawPolicy := enrollmentContextFixture(t)
			var cfg config.LocalConfigV2
			json.Unmarshal(rawPolicy, &cfg)
			cfg.SourceOrigin = s.URL
			cfg.RegistrarOrigin = s.URL
			rawPolicy, _ = json.Marshal(cfg)
			os.WriteFile(policyPath, rawPolicy, 0600)
			if mode == "legacy-saved" || mode == "routes-saved" {
				d.mu.Lock()
				if mode == "legacy-saved" {
					d.state.AuthKey = "saved-legacy-auth"
				} else {
					d.state.ExitNode = true
				}
				err := d.saveStateLocked()
				d.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
			}
			launch, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if invokeNativeCallback(t, launch) != 200 {
				t.Fatal("callback")
			}
			var saved struct {
				Credential string
				Allowed    []contract.SourceMapping
			}
			raw, _ := os.ReadFile(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending", "enrollment-pending-v1.json"))
			if json.Unmarshal(raw, &saved) != nil {
				t.Fatal("intent")
			}
			token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{13}, 32))
			own := contract.SourceExchange{ResponseVersion: 2, Provenance: contract.SourceProvenance{PrincipalAuthority: cfg.PrincipalAuthority, PrincipalNamespace: cfg.PrincipalNamespace, PrincipalID: "local-user", SessionBinding: token}, Launch: launch, Ticket: token, ID: token, ExpiresAt: time.Now().Add(30 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"), Mapping: saved.Allowed[0], Purpose: "mnet-enrollment"}
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(own)
			}))
			if _, err := d.exchangeNativeEnrollment(context.Background(), source.Limits{Timeout: time.Second, MaxConnections: 1}); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(d.stateOwner.path)
			var begins, submits atomic.Int32
			var server *proof.Server
			var challenge contract.Challenge
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != registrar.WorkflowScheme+saved.Credential {
					t.Error("wrong credential")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				switch r.URL.Path {
				case registrar.WorkflowPrefix + "begin":
					begins.Add(1)
					var req contract.WorkflowBeginWire
					json.NewDecoder(r.Body).Decode(&req)
					if req.Request.Launch != launch || req.Request.Mapping != own.Mapping || req.Request.Ticket != own.Ticket {
						t.Error("not same own intent")
					}
					now := time.Now()
					b, err := registrar.OwnWorkflowBindingDATA(own, cfg.ServiceID, req.Request.ReserveID, now.UnixMilli(), now.Add(20*time.Second).UnixMilli(), now.Add(5*time.Second).UnixMilli())
					if err != nil {
						t.Error(err)
					}
					server, challenge, err = proof.NewServer(b, now, now.Add(5*time.Second), 10*time.Second)
					if err != nil {
						t.Error(err)
					}
					switch mode {
					case "wrong-principal":
						challenge.Binding.PrincipalID = "different"
					case "wrong-key":
						challenge.Binding.DevicePublicKey = token
					case "wrong-session":
						challenge.Binding.SessionBinding = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
					case "wrong-descriptor":
						challenge.Binding.DescriptorDigest = token
					case "policy-after-begin":
						os.WriteFile(policyPath, append(rawPolicy, ' '), 0600)
					case "state-after-begin":
						d.mu.Lock()
						d.state.NodeName = "changed"
						d.mu.Unlock()
					case "close-after-begin":
						d.Close()
					}
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: challenge})
				case registrar.WorkflowPrefix + "submit":
					submits.Add(1)
					phase, _ := d.enrollmentPreparation.journal.Status()
					if phase != "workflow-submit-may-have-started" {
						t.Error("no durable submit")
					}
					var req contract.WorkflowSubmitWire
					json.NewDecoder(r.Body).Decode(&req)
					response, err := base64.RawURLEncoding.Strict().DecodeString(req.ResponseMAC)
					if err != nil || len(response) != 32 {
						t.Error("MAC grammar")
						return
					}
					var mac [32]byte
					copy(mac[:], response)
					confirmation, err := server.Verify(req.ChallengeID, mac, time.Now())
					if err != nil {
						t.Error("actual durable native key not proved", err)
						return
					}
					if mode == "policy-on-submit" {
						os.WriteFile(policyPath, append(rawPolicy, ' '), 0600)
					}
					if mode == "lost-submit" {
						conn, _, _ := w.(http.Hijacker).Hijack()
						conn.Close()
						return
					}
					if mode == "wrong-confirmation" {
						confirmation[0] ^= 1
					}
					receipt, err := contract.ReceiptV2ForChallenge(challenge, "actual-key-fixture-node", token, time.Now().UnixMilli())
					if err != nil {
						t.Error(err)
					}
					json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt), ConfirmationMAC: base64.RawURLEncoding.EncodeToString(confirmation[:])})
				default:
					t.Error("wrong origin route")
					w.WriteHeader(404)
				}
			}))
			switch mode {
			case "policy-before":
				os.WriteFile(policyPath, append(rawPolicy, ' '), 0600)
			case "state-before":
				d.mu.Lock()
				d.state.NodeName = "changed"
				d.mu.Unlock()
			case "legacy-before":
				d.mu.Lock()
				d.state.AuthKey = "must-not-inherit"
				d.mu.Unlock()
			}
			result, err := d.proveNativeEnrollment(context.Background(), nativeWorkflowLimits())
			if server != nil {
				server.Close()
			}
			if mode == "success" {
				if err != nil || result.Receipt.NodeID != "actual-key-fixture-node" || submits.Load() != 1 {
					t.Fatal("real native proof failed", err, begins.Load(), submits.Load())
				}
			} else if err == nil || result != (contract.WorkflowReceiptWire{}) {
				t.Fatal("negative returned receipt", err)
			}
			expectedSubmit := int32(0)
			if mode == "success" || mode == "wrong-confirmation" || mode == "policy-on-submit" || mode == "lost-submit" {
				expectedSubmit = 1
			}
			if submits.Load() != expectedSubmit {
				t.Fatal("unexpected actual key submission", mode, submits.Load())
			}
			if mode == "policy-before" || mode == "state-before" || mode == "legacy-before" || mode == "legacy-saved" || mode == "routes-saved" {
				if begins.Load() != 0 {
					t.Fatal("changed native owner disclosed intent")
				}
			}
			priorBegin, priorSubmit := begins.Load(), submits.Load()
			if _, err := d.proveNativeEnrollment(context.Background(), nativeWorkflowLimits()); err == nil || begins.Load() != priorBegin || submits.Load() != priorSubmit {
				t.Fatal("replayed workflow/key")
			}
			after, _ := os.ReadFile(d.stateOwner.path)
			if !bytes.Equal(before, after) || d.state.NodeID != "" || d.state.NodeToken != "" || d.cc != nil || d.dev != nil {
				t.Fatal("proof DATA installed native state or tunnel")
			}
		})
	}
}
