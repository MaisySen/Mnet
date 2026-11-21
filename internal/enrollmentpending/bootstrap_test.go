package enrollmentpending

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
)

func TestBootstrapJournalFreshTLSProcess(t *testing.T) {
	if os.Getenv("MNET_BOOTSTRAP_JOURNAL_CHILD") != "" {
		bootstrapJournalTLSCases(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBootstrapJournalFreshTLSProcess$", "-test.v", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "MNET_BOOTSTRAP_JOURNAL_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh verifiedTLS fixture failed: %v\n%s", err, output)
	}
}
func bootstrapReady(t *testing.T, origin string) (*Journal, contract.Binding, SourceConfig) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pending")
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	now := time.Now()
	mapping := contract.SourceMapping{WorkspaceID: "org_test", Issuer: "https://issuer.test/path", RegistrarOrigin: origin, Audience: "fixture", MappingRevision: "r1", NetworkID: "net_test"}
	policy := fixturePolicy(t, "https://source.test", mapping)
	launch, err := j.Begin(code(), "linux", "http://127.0.0.1:34567/mnet/enrollment/callback", policy, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := j.RecordCallback(launch.AttemptID, launch.State, code(), now); err != nil {
		t.Fatal(err)
	}
	if err := j.MarkExchangeStarted(now); err != nil {
		t.Fatal(err)
	}
	config := savedConfig(t, j)
	if err := j.RecordExchange(exchange(t, j, now), config, now); err != nil {
		t.Fatal(err)
	}
	var own contract.SourceExchange
	json.Unmarshal([]byte(j.data.ExchangeJSON), &own)
	descriptor, err := contract.SourceDescriptorDigest(own, j.data.Policy.ServiceID, code())
	if err != nil {
		t.Fatal(err)
	}
	b := contract.Binding{PrincipalAuthority: own.Provenance.PrincipalAuthority, PrincipalNamespace: own.Provenance.PrincipalNamespace, PrincipalID: own.Provenance.PrincipalID, SessionBinding: own.Provenance.SessionBinding, IssuerLabel: mapping.Issuer, RegistrarOrigin: origin, Audience: mapping.Audience, ServiceID: j.data.Policy.ServiceID, WorkspaceID: mapping.WorkspaceID, NetworkID: mapping.NetworkID, MappingRevision: mapping.MappingRevision, HandoffID: own.ID, AttemptID: launch.AttemptID, ReserveID: code(), DescriptorDigest: descriptor, DescriptorEncodingVersion: 1, DevicePublicKey: launch.DevicePublicKey, CredentialDigest: launch.CredentialDigest, CheckedAtMS: now.UnixMilli(), ReserveDeadlineMS: now.Add(time.Minute).UnixMilli(), AccessUntilMS: now.Add(40 * time.Second).UnixMilli()}
	return j, b, config
}
func bootstrapJournalTLSCases(t *testing.T) {
	var active atomic.Value
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { active.Load().(http.HandlerFunc)(w, r) }))
	defer s.Close()
	root := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", root)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	for _, kind := range []string{"success", "fixed-clock", "backward-clock", "marker-fail", "lost-response", "save-fail", "policy-change", "principal-inflight", "postsave-principal", "close-inflight"} {
		t.Run(kind, func(t *testing.T) {
			j, b, cfg := bootstrapReady(t, s.URL)
			dir := j.owner.Parent()
			secret := j.data.Credential
			var sends atomic.Int64
			var cfgMu sync.Mutex
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				raw, err := os.ReadFile(j.path)
				if err != nil {
					t.Error(err)
					return
				}
				var saved record
				if !strictRecord(raw, &saved) || !validRecord(saved) || saved.Version != 3 || saved.Phase != "bootstrap-may-have-started" || saved.Credential != secret {
					t.Error("bootstrap I/O before exact durable marker")
				}
				if r.URL.Path != "/api/mnet-registrar-experimental-v4/challenge" || r.Header.Get("Authorization") != "Mnet-Enrollment-Bootstrap-V4 "+secret {
					t.Error("wrong exact saved private request")
				}
				switch kind {
				case "lost-response":
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				case "save-fail":
					j.mu.Lock()
					j.fs.rename = func(string, string) error { return injected }
					j.mu.Unlock()
				case "policy-change":
					cfgMu.Lock()
					cfg = SourceConfig{}
					cfgMu.Unlock()
				case "principal-inflight":
					cfgMu.Lock()
					cfg = changedPolicy(t, cfg, func(p *config.LocalConfigV2) { p.Bindings[0].PrincipalIDs = []string{"other"} })
					cfgMu.Unlock()
				case "close-inflight":
					j.Close()
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: contract.Challenge{Binding: b, ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: time.Now().Add(15 * time.Second).UnixMilli()}})
			}))
			if kind == "marker-fail" {
				j.fs.rename = func(string, string) error { return injected }
			}
			reads := 0
			clockReads := 0
			fixedNow := time.Now()
			clock := func() time.Time {
				clockReads++
				if kind == "fixed-clock" {
					return fixedNow
				}
				if kind == "backward-clock" && clockReads > 1 {
					return fixedNow.Add(-time.Second)
				}
				return time.Now()
			}
			challenge, err := j.BootstrapChallengeDATA(context.Background(), b, func() SourceConfig {
				cfgMu.Lock()
				defer cfgMu.Unlock()
				if kind == "postsave-principal" {
					reads++
					if reads >= 3 {
						return changedPolicy(t, cfg, func(p *config.LocalConfigV2) { p.Bindings[0].PrincipalIDs = []string{"other"} })
					}
				}
				return cfg
			}, clock, registrar.Limits{Timeout: time.Second, MaxChallengeLifetime: time.Minute})
			if kind == "success" || kind == "fixed-clock" {
				if err != nil || challenge.Binding != b {
					t.Fatal("saved challenge DATA", err)
				}
				raw, _ := os.ReadFile(j.path)
				if !bytes.Contains(raw, []byte(`bootstrap-challenge-data`)) {
					t.Fatal("returned before durable challenge pin")
				}
			} else if err == nil || challenge != (contract.Challenge{}) {
				t.Fatal("failed challenge returned DATA", kind, err)
			}
			if kind == "marker-fail" && sends.Load() != 0 {
				t.Fatal("send before marker")
			}
			if kind != "marker-fail" && sends.Load() != 1 {
				t.Fatal("unexpected send count")
			}
			if _, err := j.BootstrapChallengeDATA(context.Background(), b, func() SourceConfig { cfgMu.Lock(); defer cfgMu.Unlock(); return cfg }, time.Now, registrar.Limits{Timeout: time.Second, MaxChallengeLifetime: time.Minute}); err == nil {
				t.Fatal("spent request automatically repeated")
			}
			j.Close()
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal("durable historical image refused", err)
			}
			defer reopened.Close()
			phase, err := reopened.Status()
			if err != nil {
				t.Fatal(err)
			}
			expected := "bootstrap-may-have-started"
			if kind == "success" || kind == "fixed-clock" || kind == "postsave-principal" {
				expected = "bootstrap-challenge-data"
			}
			if kind == "marker-fail" {
				expected = "exchanged-data"
			}
			if phase != expected {
				t.Fatal("wrong durable outcome", phase, expected)
			}
			if _, err := reopened.BootstrapChallengeDATA(context.Background(), b, func() SourceConfig { cfgMu.Lock(); defer cfgMu.Unlock(); return cfg }, time.Now, registrar.Limits{Timeout: time.Second, MaxChallengeLifetime: time.Minute}); err == nil {
				t.Fatal("reopen automatically resumed challenge")
			}
			if kind != "marker-fail" && reopened.data.Credential != secret {
				t.Fatal("credential rotated")
			}
		})
	}
	bootstrapFaultMatrix(t, s, &active)

}
func TestBootstrapJournalBindingAndOldShapeBoundaries(t *testing.T) {
	j, b, cfg := bootstrapReady(t, "https://registrar.invalid")
	var cfgMu sync.Mutex
	before, _ := os.ReadFile(j.path)
	var object map[string]json.RawMessage
	json.Unmarshal(before, &object)
	object["bootstrap_json"] = json.RawMessage(`""`)
	extra, _ := json.Marshal(object)
	var rejected record
	if strictRecord(extra, &rejected) {
		t.Fatal("v2 accepted extra bootstrap field even empty")
	}
	if bytes.Contains(before, []byte("bootstrap_json")) {
		t.Fatal("v2 byte shape gained bootstrap field before explicit transition")
	}
	for _, kind := range []string{"principal", "session", "key", "attempt", "service", "descriptor", "mapping"} {
		changed := b
		switch kind {
		case "principal":
			changed.PrincipalID += "x"
		case "session":
			changed.SessionBinding = changed.AttemptID
		case "key":
			changed.DevicePublicKey = changed.AttemptID
		case "attempt":
			changed.AttemptID = changed.DevicePublicKey
		case "service":
			changed.ServiceID += "x"
		case "descriptor":
			changed.DescriptorDigest = changed.DevicePublicKey
		case "mapping":
			changed.NetworkID += "x"
		}
		if _, err := j.BootstrapChallengeDATA(context.Background(), changed, func() SourceConfig { cfgMu.Lock(); defer cfgMu.Unlock(); return cfg }, time.Now, registrar.Limits{Timeout: time.Second, MaxChallengeLifetime: time.Minute}); !errors.Is(err, ErrInvalid) {
			t.Fatal("changed saved binding accepted", kind, err)
		}
		after, _ := os.ReadFile(j.path)
		if !bytes.Equal(before, after) {
			t.Fatal("invalid binding mutated journal")
		}
	}
	revoked := changedPolicy(t, cfg, func(p *config.LocalConfigV2) { p.Bindings[0].PrincipalIDs = []string{"another-person"} })
	if _, err := j.BootstrapChallengeDATA(context.Background(), b, func() SourceConfig { return revoked }, time.Now, registrar.Limits{Timeout: time.Second, MaxChallengeLifetime: time.Minute}); err != ErrInvalid {
		t.Fatal("current principal removal admitted challenge request", err)
	}
	afterRevocation, _ := os.ReadFile(j.path)
	if !bytes.Equal(before, afterRevocation) {
		t.Fatal("principal revocation changed journal")
	}
	// Existing v2 shape still loads. No silent migration by Open/Status.
	dir := j.owner.Parent()
	j.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopened.Status()
	after, _ := os.ReadFile(reopened.path)
	if !bytes.Equal(before, after) {
		t.Fatal("Open migrated v2")
	}
	if strings.Contains(string(after), "bootstrap_json") {
		t.Fatal("extra bootstrap file field added")
	}
}

func bootstrapFaultMatrix(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, when := range []string{"marker", "pin"} {
		for _, stage := range []string{"create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync"} {
			t.Run(when+"-"+stage, func(t *testing.T) {
				j, b, cfg := bootstrapReady(t, s.URL)
				dir := j.owner.Parent()
				before, _ := os.ReadFile(j.path)
				var sends atomic.Int64
				inject := func() {
					real := j.fs
					j.fs.create = func(dir string) (tempFile, error) {
						if stage == "create" {
							return nil, injected
						}
						f, err := real.create(dir)
						if err != nil {
							return nil, err
						}
						return &wrappedTemp{f, stage}, nil
					}
					if stage == "rename" {
						j.fs.rename = func(string, string) error { return injected }
					}
					if stage == "after-rename" {
						j.fs.rename = func(a, b string) error {
							if err := real.rename(a, b); err != nil {
								return err
							}
							return injected
						}
					}
					if stage == "dirsync" {
						j.fs.syncDir = func(string) error { return injected }
					}
				}
				active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					sends.Add(1)
					if when == "pin" {
						j.mu.Lock()
						inject()
						j.mu.Unlock()
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: contract.Challenge{Binding: b, ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: time.Now().Add(10 * time.Second).UnixMilli()}})
				}))
				if when == "marker" {
					inject()
				}
				got, err := j.BootstrapChallengeDATA(context.Background(), b, func() SourceConfig { return cfg }, time.Now, registrar.Limits{Timeout: time.Second, MaxChallengeLifetime: time.Minute})
				if err == nil || got != (contract.Challenge{}) {
					t.Fatal("persistence failure returned DATA")
				}
				uncertain := stage == "after-rename" || stage == "dirsync"
				if uncertain && !errors.Is(err, ErrUncertain) {
					t.Fatal("lost local uncertainty classification", err)
				}
				if when == "marker" {
					if sends.Load() != 0 {
						t.Fatal("I/O before durable marker")
					}
					if !uncertain {
						after, _ := os.ReadFile(j.path)
						if !bytes.Equal(before, after) || j.data.Version != 2 {
							t.Fatal("rollback-safe marker mutated v2")
						}
					}
				} else if sends.Load() != 1 || !errors.Is(err, ErrRemoteUnknown) {
					t.Fatal("pin failure lost remote unknown", err)
				}
				j.Close()
				next, err := Open(dir)
				if err != nil {
					t.Fatal("owning reopen failed", err)
				}
				defer next.Close()
				phase, _ := next.Status()
				expected := "exchanged-data"
				if when == "marker" && uncertain || when == "pin" {
					expected = "bootstrap-may-have-started"
				}
				if when == "pin" && uncertain {
					expected = "bootstrap-challenge-data"
				}
				if phase != expected {
					t.Fatal("wrong whole-image crash outcome", phase, expected)
				}
			})
		}
	}
}

func TestBootstrapRecordExactGrammarAndNoRewrite(t *testing.T) {
	j, b, _ := bootstrapReady(t, "https://registrar.invalid")
	r := *j.data
	dir := j.owner.Parent()
	path := j.path
	r.Version = 3
	r.Phase = "bootstrap-may-have-started"
	data := bootstrapData{Binding: b, StartedMS: time.Now().UnixMilli(), TimeoutMS: 1000, MaxLifetimeMS: 60000}
	encode := func(v bootstrapData) string { raw, _ := json.Marshal(v); return string(raw) }
	r.BootstrapJSON = encode(data)
	if !validRecord(r) {
		t.Fatal("fixture invalid")
	}
	cases := map[string]func(*record){
		"missing":        func(r *record) { r.BootstrapJSON = "" },
		"null":           func(r *record) { r.BootstrapJSON = "null" },
		"unknown":        func(r *record) { r.BootstrapJSON = strings.Replace(r.BootstrapJSON, "{", `{"unknown":true,`, 1) },
		"duplicate":      func(r *record) { r.BootstrapJSON = strings.Replace(r.BootstrapJSON, "{", `{"started_ms":1,`, 1) },
		"principal":      func(r *record) { d := data; d.Binding.PrincipalID = "other"; r.BootstrapJSON = encode(d) },
		"timing":         func(r *record) { d := data; d.StartedMS = r.ExpiresMS; r.BootstrapJSON = encode(d) },
		"phase":          func(r *record) { r.Phase = "exchanged-data" },
		"missing-pin":    func(r *record) { r.Phase = "bootstrap-challenge-data" },
		"unexpected-pin": func(r *record) { d := data; d.ChallengeJSON = `{}`; r.BootstrapJSON = encode(d) },
		"pin-binding": func(r *record) {
			d := data
			c := contract.Challenge{Binding: b, ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: time.Now().Add(time.Second).UnixMilli()}
			c.Binding.AttemptID = strings.Repeat("A", 43)
			raw, _ := json.Marshal(contract.ChallengeWire{Version: 4, Challenge: c})
			d.ChallengeJSON = string(raw)
			r.BootstrapJSON = encode(d)
			r.Phase = "bootstrap-challenge-data"
		},
	}
	j.Close()
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := r
			mutate(&bad)
			raw, _ := json.Marshal(bad)
			var decoded record
			if strictRecord(raw, &decoded) && validRecord(decoded) {
				t.Fatal("malformed historical v3 accepted")
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := Open(dir)
			if err == nil {
				opened.Close()
				t.Fatal("malformed file accepted")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatal("malformed record rewritten")
			}
		})
	}
}
