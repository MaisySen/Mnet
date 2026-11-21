package enrollmentpending

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
)

func workflowLimits() registrar.WorkflowLimits {
	return registrar.WorkflowLimits{Timeout: time.Second, MaxChallengeLifetime: 10 * time.Second, MaxReserveLifetime: time.Minute, Timing: contract.FixtureTiming{MaxObservationAgeMS: 20000, MaxLeaseMS: 10000, ClockUncertaintyMS: 1}}
}
func TestWorkflowJournalFreshTLSChild(t *testing.T) {
	if os.Getenv("MNET_WORKFLOW_JOURNAL_CHILD") == "1" {
		workflowJournalCases(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkflowJournalFreshTLSChild$", "-test.v", "-test.timeout=90s")
	cmd.Env = append(os.Environ(), "MNET_WORKFLOW_JOURNAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("journal workflow child: %v\n%s", err, out)
	}
}
func workflowJournalCases(t *testing.T) {
	var active atomic.Value
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { active.Load().(http.HandlerFunc)(w, r) }))
	defer s.Close()
	root := filepath.Join(t.TempDir(), "cert.pem")
	os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", root)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	for _, mode := range []string{"success", "wrong-principal", "lost-begin", "lost-submit", "begin-marker-fail", "challenge-pin-fail", "submit-marker-fail", "receipt-pin-fail", "principal-after-begin", "postpin-principal", "close-begin"} {
		t.Run(mode, func(t *testing.T) {
			j, _, cfg := bootstrapReady(t, s.URL)
			dir := j.owner.Parent()
			secret := j.data.Credential
			var sends int
			var c contract.Challenge
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends++
				raw, _ := os.ReadFile(j.path)
				var saved record
				if !strictRecord(raw, &saved) || !validRecord(saved) || saved.Version != 4 {
					t.Error("I/O without same durable valid v4 marker")
				}
				if r.Header.Get("Authorization") != registrar.WorkflowScheme+secret {
					t.Error("wrong journal private credential")
				}
				switch r.URL.Path {
				case registrar.WorkflowPrefix + "begin":
					if saved.Phase != "workflow-begin-may-have-started" {
						t.Error("no durable begin marker")
					}
					var input contract.WorkflowBeginWire
					json.NewDecoder(r.Body).Decode(&input)
					own, err := contract.DecodeSourceExchange([]byte(saved.ExchangeJSON), saved.Launch, saved.Authority, saved.Allowed, saved.Allowed, time.Now())
					if err != nil {
						t.Error(err)
					}
					descriptor, _ := contract.SourceDescriptorDigest(own, saved.Policy.ServiceID, input.Request.ReserveID)
					p, m := own.Provenance, own.Mapping
					now := time.Now().UnixMilli()
					c = contract.Challenge{ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: now + 5000, Binding: contract.Binding{PrincipalAuthority: p.PrincipalAuthority, PrincipalNamespace: p.PrincipalNamespace, PrincipalID: p.PrincipalID, SessionBinding: p.SessionBinding, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, ServiceID: saved.Policy.ServiceID, WorkspaceID: m.WorkspaceID, NetworkID: m.NetworkID, MappingRevision: m.MappingRevision, HandoffID: own.ID, AttemptID: own.Launch.AttemptID, ReserveID: input.Request.ReserveID, DescriptorDigest: descriptor, DescriptorEncodingVersion: 1, DevicePublicKey: own.Launch.DevicePublicKey, CredentialDigest: own.Launch.CredentialDigest, CheckedAtMS: now, ReserveDeadlineMS: now + 30000, AccessUntilMS: now + 5000}}
					if mode == "wrong-principal" {
						c.Binding.PrincipalID = "not-own-person"
					}
					if mode == "challenge-pin-fail" {
						j.mu.Lock()
						j.fs.rename = func(string, string) error { return injected }
						j.mu.Unlock()
					}
					if mode == "close-begin" {
						j.Close()
					}
					if mode == "lost-begin" {
						conn, _, _ := w.(http.Hijacker).Hijack()
						conn.Close()
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
				case registrar.WorkflowPrefix + "submit":
					if saved.Phase != "workflow-submit-may-have-started" {
						t.Error("submit preceded durable marker")
					}
					if mode == "receipt-pin-fail" {
						j.mu.Lock()
						j.fs.rename = func(string, string) error { return injected }
						j.mu.Unlock()
					}
					if mode == "lost-submit" {
						conn, _, _ := w.(http.Hijacker).Hijack()
						conn.Close()
						return
					}
					receipt, err := contract.ReceiptV2ForChallenge(c, "node-fixture", code(), time.Now().UnixMilli())
					if err != nil {
						t.Error(err)
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt), ConfirmationMAC: code()})
				default:
					t.Error("old transport or arbitrary path used")
					w.WriteHeader(404)
				}
			}))
			if mode == "begin-marker-fail" {
				j.fs.rename = func(string, string) error { return injected }
			}
			reads := 0
			challenge, err := j.BeginWorkflowDATA(context.Background(), func() SourceConfig {
				reads++
				if mode == "postpin-principal" && reads >= 3 {
					return SourceConfig{}
				}
				return cfg
			}, workflowLimits())
			beginOK := mode == "success" || mode == "lost-submit" || mode == "submit-marker-fail" || mode == "receipt-pin-fail" || mode == "principal-after-begin"
			if beginOK {
				if err != nil || challenge != c {
					t.Fatal("durable begin", err)
				}
				if mode == "submit-marker-fail" {
					j.fs.rename = func(string, string) error { return injected }
				}
				if mode == "principal-after-begin" {
					cfg = SourceConfig{}
				}
				var response [32]byte
				response[0] = 1
				receipt, submitErr := j.SubmitWorkflowDATA(context.Background(), challenge.ID, response, func() SourceConfig { return cfg })
				if mode == "success" {
					if submitErr != nil || receipt.Receipt.ChallengeID != c.ID {
						t.Fatal("exact receipt delivery", submitErr)
					}
				} else if submitErr == nil || receipt != (contract.WorkflowReceiptWire{}) {
					t.Fatal("failed submit returned receipt")
				}
			} else if err == nil || challenge != (contract.Challenge{}) {
				t.Fatal("failed begin returned challenge", mode)
			}
			if mode == "begin-marker-fail" && sends != 0 {
				t.Fatal("marker fail sent secret")
			}
			if _, err := j.BeginWorkflowDATA(context.Background(), func() SourceConfig { return cfg }, workflowLimits()); err == nil {
				t.Fatal("begin automatically retried")
			}
			j.Close()
			reopen, err := Open(dir)
			if err != nil {
				t.Fatal("durable history refused", err)
			}
			defer reopen.Close()
			phase, _ := reopen.Status()
			expected := "workflow-begin-may-have-started"
			switch mode {
			case "success":
				expected = "workflow-receipt-data"
			case "begin-marker-fail":
				expected = "exchanged-data"
			case "lost-submit", "receipt-pin-fail":
				expected = "workflow-submit-may-have-started"
			case "submit-marker-fail", "principal-after-begin", "postpin-principal":
				expected = "workflow-challenge-data"
			}
			if phase != expected {
				t.Fatal("wrong durable phase", phase, expected)
			}
			if _, err := reopen.BeginWorkflowDATA(context.Background(), func() SourceConfig { return cfg }, workflowLimits()); err == nil {
				t.Fatal("reopen resumed network")
			}
		})
	}
	workflowJournalFaults(t, s, &active)
	workflowKeyJournalCases(t, s, &active)
	workflowFinalizeJournalCases(t, s, &active)

}
func TestWorkflowChallengeOwnFieldsIndependent(t *testing.T) {
	j, b, _ := bootstrapReady(t, "https://registrar.invalid")
	var own contract.SourceExchange
	json.Unmarshal([]byte(j.data.ExchangeJSON), &own)
	now := time.Now().UnixMilli()
	b.CheckedAtMS = now
	b.ReserveDeadlineMS = now + 30000
	b.AccessUntilMS = now + 5000
	c := contract.Challenge{Binding: b, ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: now + 5000}
	if registrar.CheckWorkflowChallenge(c, own, j.data.Policy.ServiceID, b.ReserveID, workflowLimits(), time.Now()) != nil {
		t.Fatal("fixture wrong")
	}
	for _, kind := range []string{"principal", "session", "service", "descriptor", "key", "credential", "future", "long-reserve", "long-lease"} {
		changed := c
		switch kind {
		case "principal":
			changed.Binding.PrincipalID += "x"
		case "session":
			changed.Binding.SessionBinding = strings.Repeat("A", 43)
		case "service":
			changed.Binding.ServiceID += "x"
		case "descriptor":
			changed.Binding.DescriptorDigest = strings.Repeat("A", 43)
		case "key":
			changed.Binding.DevicePublicKey = strings.Repeat("A", 43)
		case "credential":
			changed.Binding.CredentialDigest = strings.Repeat("A", 43)
		case "future":
			changed.Binding.CheckedAtMS = now + 1000
		case "long-reserve":
			changed.Binding.ReserveDeadlineMS = now + 120000
		case "long-lease":
			changed.Binding.AccessUntilMS = now + 20000
		}
		if registrar.CheckWorkflowChallenge(changed, own, j.data.Policy.ServiceID, b.ReserveID, workflowLimits(), time.Now()) == nil {
			t.Fatal("independent check ignored", kind)
		}
	}
	before, _ := os.ReadFile(j.path)
	var object map[string]json.RawMessage
	json.Unmarshal(before, &object)
	object["workflow_json"] = json.RawMessage(`""`)
	raw, _ := json.Marshal(object)
	var r record
	if strictRecord(raw, &r) {
		t.Fatal("old v2 shape widened")
	}
	var response [32]byte
	response[0] = 1
	if registrar.WorkflowResponseDigest(response) == base64.RawURLEncoding.EncodeToString(response[:]) {
		t.Fatal("response digest is raw MAC")
	}
	after, _ := os.ReadFile(j.path)
	if !bytes.Equal(before, after) {
		t.Fatal("validation changed journal")
	}
}
