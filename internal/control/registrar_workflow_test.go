package control

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
)

func TestRegistrarWorkflowFreshTLSChild(t *testing.T) {
	if os.Getenv("MNET_WORKFLOW_CHILD") == "1" {
		registrarWorkflowCases(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRegistrarWorkflowFreshTLSChild$", "-test.v", "-test.timeout=90s")
	cmd.Env = append(os.Environ(), "MNET_WORKFLOW_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("workflow child: %v\n%s", err, out)
	}
}

type workflowFixture struct {
	s        *Store
	request  contract.SourceReserveRequest
	secret   string
	private  [32]byte
	policy   registrarWorkflowConfig
	observed contract.SourceReserve
}

func newWorkflowFixture(t *testing.T, origin string) *workflowFixture {
	t.Helper()
	s, err := openTestStore(t, filepath.Join(t.TempDir(), "control.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := s.CreateNetwork("development")
	if err != nil {
		t.Fatal(err)
	}
	c, req, observed := storeSourceFixture(t, origin)
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := registrarContractDigest(57)
	sum := sha256.Sum256([]byte(secret))
	req.Launch.DevicePublicKey = base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
	req.Launch.CredentialDigest = base64.RawURLEncoding.EncodeToString(sum[:])
	req.Mapping.NetworkID = network.ID
	c.Sources[0].Allowed = []contract.SourceMapping{req.Mapping}
	writeStoreSource(t, s, c)
	m := req.Mapping
	p := observed.Provenance
	policy := registrarWorkflowConfig{Version: 1, Mode: "development", Revision: "workflow-fixture-r1", SourceID: "fixture", Policy: config.LocalConfigV2{Version: 2, Revision: m.MappingRevision, SourceOrigin: origin, PrincipalAuthority: p.PrincipalAuthority, PrincipalNamespace: p.PrincipalNamespace, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, ServiceID: c.Sources[0].ServiceID, Bindings: []config.LocalBindingV2{{WorkspaceID: m.WorkspaceID, NetworkID: m.NetworkID, PrincipalIDs: []string{p.PrincipalID}}}}, ChallengeMS: 10000, MaxLeaseMS: 15000, MaxObservationAgeMS: 20000, ClockUncertaintyMS: 1, MaxLivePerPrincipal: 4, MaxProofAttempts: 4, MaxLive: 4, MaxAttempts: 8, MaxRecords: 8, MaxStateBytes: 1024 * 1024}
	raw, _ := json.Marshal(policy)
	if err := os.WriteFile(s.ownedPath+".registrar-workflow-v1.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	observed.Deadline = time.Now().Add(25 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z")
	observed.Launch = req.Launch
	observed.Mapping = req.Mapping
	observed.DescriptorDigest, _ = contract.ServiceReserveDescriptorDigest(req, policy.Policy.ServiceID)
	f := &workflowFixture{s: s, request: req, secret: secret, policy: policy, observed: observed}
	copy(f.private[:], private.Bytes())
	return f
}
func (f *workflowFixture) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mnet-enrollment-service-v1/reserve-evidence-v2" || r.Header.Get("Authorization") != "Bearer fixture-key."+base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32)) {
			t.Error("wrong source endpoint/secret")
			w.WriteHeader(403)
			return
		}
		var got contract.SourceReserveRequest
		if json.NewDecoder(r.Body).Decode(&got) != nil || got != f.request {
			t.Error("source request changed")
			w.WriteHeader(400)
			return
		}
		observed := f.observed
		observed.CheckedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(observed)
	}
}
func (f *workflowFixture) respond(t *testing.T, c contract.Challenge) (*proof.Native, [32]byte) {
	t.Helper()
	n, err := proof.Respond(f.private, c, c.Binding, time.Now(), time.Duration(f.policy.ChallengeMS)*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	response, err := n.Response(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return n, response
}
func registrarWorkflowCases(t *testing.T) {
	var active atomic.Value
	var calls atomic.Int64
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); active.Load().(http.HandlerFunc)(w, r) }))
	defer source.Close()
	root := filepath.Join(t.TempDir(), "fixture.pem")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", root)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	t.Run("connected-owned-source-proof-atomic-receipt-map", func(t *testing.T) {
		f := newWorkflowFixture(t, source.URL)
		active.Store(f.handler(t))
		beforeCalls := calls.Load()
		before, _ := os.ReadFile(f.s.path)
		for _, secret := range []string{"", f.request.Launch.CredentialDigest, registrarContractDigest(58)} {
			if _, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, secret); err == nil {
				t.Fatal("bad bootstrap accepted")
			}
		}
		if calls.Load() != beforeCalls {
			t.Fatal("unauthenticated source send")
		}
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal("begin", err)
		}
		again, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil || again != c || calls.Load() != beforeCalls+1 {
			t.Fatal("challenge retry changed source/identity", err)
		}
		after, _ := os.ReadFile(f.s.path)
		if !bytes.Equal(before, after) || len(f.s.data.Nodes) != 0 || f.s.data.Registrar != nil {
			t.Fatal("challenge granted Store authority")
		}
		n, response := f.respond(t, c)
		wrong := response
		wrong[0] ^= 1
		for _, test := range []struct {
			id, secret string
			response   [32]byte
		}{{"other", f.secret, response}, {c.ID, f.request.Launch.CredentialDigest, response}, {c.ID, f.secret, wrong}} {
			if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, test.id, test.secret, test.response); err == nil {
				t.Fatal("wrong proof accepted")
			}
		}
		result, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response)
		if err != nil {
			t.Fatal("commit", err)
		}
		if n.Confirm(result.Confirmation, time.Now()) != nil {
			t.Fatal("wrong owned confirmation")
		}
		if len(f.s.data.Nodes) != 1 || len(f.s.data.NodeTokens) != 0 || len(f.s.data.AuthKeys) != 0 || len(f.s.data.Registrar.Records) != 1 || len(f.s.data.Registrar.ConsumedProofs) != 1 {
			t.Fatal("atomic candidate missing or mixed legacy state")
		}
		version := f.s.Version()
		disk, _ := os.ReadFile(f.s.path)
		retry, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response)
		if err != nil || retry != result || f.s.Version() != version {
			t.Fatal("exact commit retry changed receipt", err)
		}
		diskAfter, _ := os.ReadFile(f.s.path)
		if !bytes.Equal(disk, diskAfter) {
			t.Fatal("historical retry saved")
		}
		f.s = reopenRegistrarFixture(t, f.s)
		history, err := f.s.registrarHistoricalReceipt(c, f.secret)
		if err != nil || history != result.History {
			t.Fatal("owning reopen lost receipt", err)
		}
		r := f.s.registrarNodeLocked(history.Receipt.NodeID)
		ref := registrarCredentialReference{NodeID: r.NodeID, NetworkID: r.NetworkID, DevicePublicKey: r.DevicePublicKey, Scope: r.Scope, ReceiptID: r.ReceiptID, ReceiptDigest: r.ReceiptDigest, DescriptorDigest: r.DescriptorDigest, TranscriptDigest: r.Consumption.TranscriptDigest}
		m, err := f.s.registrarCredentialMap(ref, f.secret)
		if err != nil || m.Self.ID != r.NodeID || !m.Self.Approved || m.RelayURL != "" {
			t.Fatal("actual committed current map unavailable", err)
		}
		if _, err := f.s.AuthorizeNode(r.NodeID, f.secret); err == nil {
			t.Fatal("native secret became legacy token")
		}
		if err := f.s.ExpireNode(r.NodeID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.registrarCredentialMap(ref, f.secret); err == nil {
			t.Fatal("revoked access retained")
		}
		if got, err := f.s.registrarHistoricalReceipt(c, f.secret); err != nil || got != history {
			t.Fatal("revocation erased receipt", err)
		}
		if _, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret); err == nil || calls.Load() != beforeCalls+1 {
			t.Fatal("restart/tombstone granted new source attempt")
		}
	})
	for _, stage := range []string{"create-temp", "write", "short-write", "sync-file", "close-file", "rename", "rename-after-publish", "sync-directory", "pre-publish-policy", "post-publish-policy", "post-publish-cancel", "late-rename", "during-rename-policy"} {
		t.Run(stage, func(t *testing.T) {
			f := newWorkflowFixture(t, source.URL)
			if stage == "late-rename" {
				f.policy.ChallengeMS = 250
				f.policy.MaxLeaseMS = 250
				raw, _ := json.Marshal(f.policy)
				os.WriteFile(f.s.ownedPath+".registrar-workflow-v1.json", raw, 0600)
			}
			active.Store(f.handler(t))
			c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
			if err != nil {
				t.Fatal(err)
			}
			_, response := f.respond(t, c)
			before, _ := os.ReadFile(f.s.path)
			version := f.s.Version()
			cursor, _ := json.Marshal(f.s.data.NextIP)
			real := f.s.fs
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.s.fs.createTemp = func(dir string) (storeTemp, error) {
				if stage == "create-temp" {
					return nil, errInjectedStoreIO
				}
				temp, err := real.createTemp(dir)
				if err != nil {
					return nil, err
				}
				return faultStoreTemp{temp, stage}, nil
			}
			if stage == "rename" {
				f.s.fs.rename = func(string, string) error { return errInjectedStoreIO }
			}
			if stage == "pre-publish-policy" {
				f.s.fs.createTemp = func(dir string) (storeTemp, error) {
					os.Remove(f.s.ownedPath + ".registrar-workflow-v1.json")
					return real.createTemp(dir)
				}
			}
			if stage == "during-rename-policy" {
				f.s.fs.rename = func(a, b string) error {
					os.Remove(f.s.ownedPath + ".registrar-workflow-v1.json")
					return real.rename(a, b)
				}
			}
			if stage == "late-rename" {
				f.s.fs.rename = func(a, b string) error { time.Sleep(350 * time.Millisecond); return real.rename(a, b) }
			}
			if stage == "rename-after-publish" || stage == "post-publish-policy" || stage == "post-publish-cancel" {
				f.s.fs.rename = func(a, b string) error {
					if err := real.rename(a, b); err != nil {
						return err
					}
					switch stage {
					case "post-publish-policy":
						os.Remove(f.s.ownedPath + ".registrar-workflow-v1.json")
					case "post-publish-cancel":
						cancel()
					default:
						return errInjectedStoreIO
					}
					return nil
				}
			}
			if stage == "sync-directory" {
				f.s.fs.syncDir = func(path string) error {
					if err := real.syncDir(path); err != nil {
						return err
					}
					return errInjectedStoreIO
				}
			}
			result, err := f.s.submitRegistrarEnrollment(ctx, c.Binding.AttemptID, c.ID, f.secret, response)
			if err == nil || result != (registrarEnrollmentResult{}) {
				t.Fatal("failed publication returned receipt")
			}
			uncertain := stage == "rename-after-publish" || stage == "sync-directory" || stage == "post-publish-policy" || stage == "post-publish-cancel" || stage == "late-rename" || stage == "during-rename-policy"
			if f.s.Version() != version {
				t.Fatal("failed publication notified")
			}
			if uncertain {
				if !errors.Is(err, ErrStoreUncertain) || f.s.Health() == nil {
					t.Fatal("late/ambiguous publication not frozen", err)
				}
			} else {
				after, _ := os.ReadFile(f.s.path)
				current, _ := json.Marshal(f.s.data.NextIP)
				if !bytes.Equal(before, after) || !bytes.Equal(cursor, current) || len(f.s.data.Nodes) != 0 || f.s.data.Registrar != nil {
					t.Fatal("rollback-safe failure changed authority")
				}
			}
			f.s = reopenRegistrarFixture(t, f.s)
			history, herr := f.s.registrarHistoricalReceipt(c, f.secret)
			if uncertain {
				if herr != nil || history.Receipt.NodeID == "" || len(f.s.data.Nodes) != 1 {
					t.Fatal("whole durable candidate missing", herr)
				}
			} else if herr == nil || len(f.s.data.Nodes) != 0 {
				t.Fatal("failed prepublication created receipt")
			}
			if stage == "late-rename" {
				r := registrarFixtureRecord(f.s)
				ref := registrarCredentialReference{NodeID: r.NodeID, NetworkID: r.NetworkID, DevicePublicKey: r.DevicePublicKey, Scope: r.Scope, ReceiptID: r.ReceiptID, ReceiptDigest: r.ReceiptDigest, DescriptorDigest: r.DescriptorDigest, TranscriptDigest: r.Consumption.TranscriptDigest}
				if _, err := f.s.authorizeRegistrarCredential(ref, f.secret); err == nil {
					t.Fatal("late rename left usable authority after reopen")
				}
			}
			if !uncertain && stage != "pre-publish-policy" {
				if _, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret); err != nil {
					t.Fatal("same reserve fresh process reconciliation failed", err)
				}
			}
		})
	}
	registrarWorkflowAdversarial(t, source.URL, &active, &calls)

}
