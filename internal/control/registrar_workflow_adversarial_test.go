package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fintex.au/mnet/internal/netmap"
	"net/http"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func registrarWorkflowAdversarial(t *testing.T, origin string, active *atomic.Value, calls *atomic.Int64) {
	t.Run("one-winner-concurrent-proof-and-immutable-history", func(t *testing.T) {
		f := newWorkflowFixture(t, origin)
		active.Store(f.handler(t))
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal(err)
		}
		_, response := f.respond(t, c)
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan registrarEnrollmentResult, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				got, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response)
				if err != nil {
					t.Error(err)
					return
				}
				results <- got
			}()
		}
		version := f.s.Version()
		close(start)
		wg.Wait()
		close(results)
		var original registrarEnrollmentResult
		for got := range results {
			if original == (registrarEnrollmentResult{}) {
				original = got
			} else if got != original {
				t.Fatal("concurrent winners differed")
			}
		}
		if len(f.s.data.Nodes) != 1 || len(f.s.data.Registrar.Records) != 1 || len(f.s.data.Registrar.ConsumedProofs) != 1 || f.s.Version() != version+1 {
			t.Fatal("duplicate node/receipt/notification")
		}
	})
	for _, kind := range []string{"off", "production-mode", "wrong-principal", "wrong-network", "state-capacity", "live-capacity", "source-loss", "source-policy-change", "cancel-inflight", "close-inflight", "stale-source", "future-source"} {
		t.Run(kind, func(t *testing.T) {
			f := newWorkflowFixture(t, origin)
			handler := f.handler(t)
			switch kind {
			case "off":
				os.Remove(f.s.ownedPath + ".registrar-workflow-v1.json")
			case "production-mode":
				f.policy.Mode = "production"
			case "wrong-principal":
				f.policy.Policy.Bindings[0].PrincipalIDs = []string{"not-source-person"}
			case "wrong-network":
				delete(f.s.data.Networks, f.request.Mapping.NetworkID)
			case "state-capacity":
				f.policy.MaxStateBytes = registrarEntryByteReserve
			case "live-capacity":
				f.policy.MaxLive = 1
				f.policy.MaxLivePerPrincipal = 1
				f.policy.MaxRecords = 1
			}
			if kind != "off" {
				raw, _ := json.Marshal(f.policy)
				os.WriteFile(f.s.ownedPath+".registrar-workflow-v1.json", raw, 0600)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "source-loss":
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				case "source-policy-change":
					os.Remove(f.s.ownedPath + ".registrar-workflow-v1.json")
				case "cancel-inflight":
					cancel()
				case "close-inflight":
					f.s.Close()
				case "stale-source", "future-source":
					observed := f.observed
					at := time.Now().Add(-21 * time.Second)
					if kind == "future-source" {
						at = time.Now().Add(time.Minute)
					}
					observed.CheckedAt = at.UTC().Format("2006-01-02T15:04:05.000Z")
					observed.Deadline = at.Add(time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					json.NewEncoder(w).Encode(observed)
					return
				}
				handler(w, r)
			}))
			beforeCalls := calls.Load()
			before, _ := os.ReadFile(f.s.path)
			c, err := f.s.beginRegistrarEnrollment(ctx, f.request, f.secret)
			if kind == "live-capacity" {
				if err != nil {
					t.Fatal(err)
				}
				other := f.request
				other.Launch.AttemptID = registrarContractDigest(66)
				other.HandoffID = registrarContractDigest(67)
				other.ReserveID = registrarContractDigest(68)
				if _, err := f.s.beginRegistrarEnrollment(context.Background(), other, f.secret); err == nil {
					t.Fatal("pending capacity not reserved")
				}
				if calls.Load() != beforeCalls+1 {
					t.Fatal("over-capacity source I/O")
				}
				return
			}
			if err == nil || c != (contract.Challenge{}) {
				t.Fatal("invalid config/source/current admitted challenge", kind, err)
			}
			after, _ := os.ReadFile(f.s.path)
			if !bytes.Equal(before, after) || len(f.s.data.Nodes) != 0 || f.s.data.Registrar != nil {
				t.Fatal("failed admission published authority")
			}
			if kind == "off" || kind == "production-mode" || kind == "wrong-network" || kind == "state-capacity" {
				if calls.Load() != beforeCalls {
					t.Fatal("failed preadmission sent source")
				}
			}
			if kind == "source-loss" || kind == "source-policy-change" {
				if _, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret); err == nil || calls.Load() != beforeCalls+1 {
					t.Fatal("unknown/reset attempt sent again")
				}
			}
		})
	}
	t.Run("policy-removal-permanently-retires-owner", func(t *testing.T) {
		f := newWorkflowFixture(t, origin)
		active.Store(f.handler(t))
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal(err)
		}
		_, response := f.respond(t, c)
		path := f.s.ownedPath + ".registrar-workflow-v1.json"
		raw, _ := os.ReadFile(path)
		os.Remove(path)
		if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); err == nil {
			t.Fatal("removed policy committed")
		}
		os.WriteFile(path, raw, 0600)
		if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); err == nil {
			t.Fatal("restored file revived old owner")
		}
		if len(f.s.data.Nodes) != 0 || f.s.data.Registrar != nil {
			t.Fatal("retirement created history")
		}
	})
	t.Run("finite-proof-budget-no-refund", func(t *testing.T) {
		f := newWorkflowFixture(t, origin)
		f.policy.MaxProofAttempts = 1
		raw, _ := json.Marshal(f.policy)
		os.WriteFile(f.s.ownedPath+".registrar-workflow-v1.json", raw, 0600)
		active.Store(f.handler(t))
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal(err)
		}
		_, response := f.respond(t, c)
		wrong := response
		wrong[0] ^= 1
		if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, wrong); err == nil {
			t.Fatal("bad response succeeded")
		}
		if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); err == nil {
			t.Fatal("failed proof budget refunded")
		}
		if len(f.s.data.Nodes) != 0 || f.s.data.Registrar != nil {
			t.Fatal("spent proof created authority")
		}
	})
	t.Run("per-principal-live-and-same-key-one-commit", func(t *testing.T) {
		f := newWorkflowFixture(t, origin)
		f.policy.MaxLivePerPrincipal = 1
		raw, _ := json.Marshal(f.policy)
		os.WriteFile(f.s.ownedPath+".registrar-workflow-v1.json", raw, 0600)
		active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req contract.SourceReserveRequest
			if json.NewDecoder(r.Body).Decode(&req) != nil {
				w.WriteHeader(400)
				return
			}
			observed := f.observed
			observed.Launch = req.Launch
			observed.HandoffID = req.HandoffID
			observed.ReserveID = req.ReserveID
			observed.DescriptorDigest, _ = contract.ServiceReserveDescriptorDigest(req, f.policy.Policy.ServiceID)
			observed.CheckedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(observed)
		}))
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal(err)
		}
		other := f.request
		other.HandoffID = registrarContractDigest(64)
		other.Launch.AttemptID = registrarContractDigest(65)
		other.ReserveID = registrarContractDigest(66)
		if _, err := f.s.beginRegistrarEnrollment(context.Background(), other, f.secret); err == nil {
			t.Fatal("principal capacity ignored")
		}
		_, response := f.respond(t, c)
		if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); err != nil {
			t.Fatal(err)
		}
		other.HandoffID = registrarContractDigest(67)
		other.Launch.AttemptID = registrarContractDigest(68)
		other.ReserveID = registrarContractDigest(69)
		beforeCalls := calls.Load()
		if _, err := f.s.beginRegistrarEnrollment(context.Background(), other, f.secret); err == nil || calls.Load() != beforeCalls {
			t.Fatal("standing same-key claim allowed another reserve")
		}
	})
	t.Run("legacy-v1-history-and-unrelated-token-key-preserved", func(t *testing.T) {
		f := newWorkflowFixture(t, origin)
		legacy, err := f.s.Join(netmap.JoinRequest{Network: f.request.Mapping.NetworkID, Name: "unrelated-legacy", PubKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{70}, 32))})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.s.ApproveNode(legacy.NodeID); err != nil {
			t.Fatal(err)
		}
		auth, err := f.s.CreateAuthKey(f.request.Mapping.NetworkID, "unchanged", true, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		old := registrarStorageFixture(t)
		record := registrarFixtureRecord(old)
		oldReceipt, _ := json.Marshal(record)
		f.s.data.Networks[record.NetworkID] = old.data.Networks[record.NetworkID]
		f.s.data.Nodes[record.NodeID] = old.data.Nodes[record.NodeID]
		f.s.data.Registrar = old.data.Registrar
		if err := f.s.saveLocked(); err != nil {
			t.Fatal(err)
		}
		active.Store(f.handler(t))
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal("mixed v1 admission", err)
		}
		_, response := f.respond(t, c)
		if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); err != nil {
			t.Fatal("v1 explicit namespace promotion", err)
		}
		after, _ := json.Marshal(record)
		if !bytes.Equal(oldReceipt, after) || f.s.data.Registrar.Version != 2 || len(f.s.data.Registrar.Records) != 2 || len(f.s.data.Registrar.ConsumedProofs) != 1 {
			t.Fatal("old history rewritten or consumption invented")
		}
		if _, err := f.s.AuthorizeNode(legacy.NodeID, legacy.NodeToken); err != nil {
			t.Fatal("unrelated legacy token changed", err)
		}
		if f.s.data.AuthKeys[auth.ID].Uses != 0 {
			t.Fatal("enrollment consumed unrelated auth key")
		}
		f.s = reopenRegistrarFixture(t, f.s)
		if _, err := f.s.registrarHistoricalReceipt(c, f.secret); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("permanent-record-capacity-no-history-eviction", func(t *testing.T) {
		f := newWorkflowFixture(t, origin)
		f.policy.MaxRecords = 1
		raw, _ := json.Marshal(f.policy)
		os.WriteFile(f.s.ownedPath+".registrar-workflow-v1.json", raw, 0600)
		active.Store(f.handler(t))
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal(err)
		}
		_, response := f.respond(t, c)
		result, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response)
		if err != nil {
			t.Fatal(err)
		}
		other := f.request
		other.HandoffID = registrarContractDigest(67)
		other.Launch.AttemptID = registrarContractDigest(68)
		other.ReserveID = registrarContractDigest(69)
		other.Launch.DevicePublicKey = registrarContractDigest(70)
		beforeCalls := calls.Load()
		if _, err := f.s.beginRegistrarEnrollment(context.Background(), other, f.secret); err == nil || calls.Load() != beforeCalls {
			t.Fatal("full history admitted new source authority")
		}
		if got, err := f.s.registrarHistoricalReceipt(c, f.secret); err != nil || got != result.History {
			t.Fatal("full capacity erased history", err)
		}
	})
	t.Run("rollback-safe-same-owned-proof-retry", func(t *testing.T) {
		f := newWorkflowFixture(t, origin)
		active.Store(f.handler(t))
		c, err := f.s.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if err != nil {
			t.Fatal(err)
		}
		_, response := f.respond(t, c)
		real := f.s.fs
		f.s.fs.rename = func(string, string) error { return errInjectedStoreIO }
		if _, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); err == nil {
			t.Fatal("failed rename succeeded")
		}
		f.s.fs = real
		result, err := f.s.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response)
		if err != nil || result.History.Receipt.NodeID == "" {
			t.Fatal("same proof was prematurely consumed", err)
		}
	})
}
func TestRegistrarWorkflowConfigExactShapeAndExplicitLimits(t *testing.T) {
	f := newWorkflowFixture(t, "https://source.test")
	raw, _ := json.Marshal(f.policy)
	if _, _, err := parseRegistrarWorkflow(raw); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing", "null", "duplicate", "case", "unknown", "float", "mode", "lease", "age", "skew", "capacity", "bytes", "source", "principal"} {
		t.Run(kind, func(t *testing.T) {
			changed := append([]byte(nil), raw...)
			switch kind {
			case "missing":
				changed = bytes.Replace(changed, []byte(`"version":1,`), nil, 1)
			case "null":
				changed = bytes.Replace(changed, []byte(`"policy":{`), []byte(`"policy":null,"unused":{`), 1)
			case "duplicate":
				changed = append([]byte(`{"version":1,`), changed[1:]...)
			case "case":
				changed = bytes.Replace(changed, []byte(`"mode"`), []byte(`"Mode"`), 1)
			case "unknown":
				changed = append([]byte(`{"extra":true,`), changed[1:]...)
			case "float":
				changed = bytes.Replace(changed, []byte(`"challenge_ms":10000`), []byte(`"challenge_ms":10000.0`), 1)
			default:
				c := f.policy
				switch kind {
				case "mode":
					c.Mode = "production"
				case "lease":
					c.MaxLeaseMS = 0
				case "age":
					c.MaxObservationAgeMS = 0
				case "skew":
					c.ClockUncertaintyMS = c.ChallengeMS
				case "capacity":
					c.MaxAttempts = 0
				case "bytes":
					c.MaxStateBytes = 0
				case "source":
					c.SourceID = ""
				case "principal":
					c.Policy.Bindings = nil
				}
				changed, _ = json.Marshal(c)
			}
			if _, _, err := parseRegistrarWorkflow(changed); err == nil {
				t.Fatal("invalid explicit policy accepted", kind)
			}
		})
	}
	value := reflect.ValueOf(f.policy)
	for i := 0; i < value.NumField(); i++ {
		var fields map[string]json.RawMessage
		json.Unmarshal(raw, &fields)
		name := value.Type().Field(i).Tag.Get("json")
		delete(fields, name)
		changed, _ := json.Marshal(fields)
		if _, _, err := parseRegistrarWorkflow(changed); err == nil {
			t.Fatal("missing policy field accepted", name)
		}
	}
}
