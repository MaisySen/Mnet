package control

import (
	"bytes"
	"encoding/json"
	"errors"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRegistrarHistoricalReceiptActualLifecycleAndReopen(t *testing.T) {
	for _, action := range []string{"live", "expired", "expire", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			r := registrarFixtureRecord(s)
			if action == "expired" {
				// Valid historical image with all exact deadlines in the past, not a
				// clock override or bypass in the production historical reader.
				delta := time.Now().Add(-time.Minute).UnixMilli() - r.AccessExpiresAtMS
				r.CommittedAtMS += delta
				r.AccessExpiresAtMS += delta
				c := &r.Consumption.Challenge
				c.ExpiresAtMS += delta
				c.Binding.CheckedAtMS += delta
				c.Binding.ReserveDeadlineMS += delta
				c.Binding.AccessUntilMS += delta
				r.Consumption.TranscriptDigest = registrarTranscriptDigest(*c)
				r.ReceiptDigest = registrarReceiptDigest(*r)
				expiry := time.UnixMilli(r.AccessExpiresAtMS)
				s.data.Nodes[r.NodeID].KeyExpiry = &expiry
				if err := s.saveLocked(); err != nil {
					t.Fatal(err)
				}
			}
			pinned := r.Consumption.Challenge
			digest := r.ReceiptDigest
			receipt, err := contract.ReceiptV2ForChallenge(pinned, r.NodeID, r.ReceiptID, r.CommittedAtMS)
			if err != nil {
				t.Fatal(err)
			}
			switch action {
			case "expire":
				if err := s.ExpireNode(r.NodeID); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := s.DeleteNode(r.NodeID); err != nil {
					t.Fatal(err)
				}
			}
			s = reopenRegistrarFixture(t, s)
			before, _ := os.ReadFile(s.path)
			memory, _ := json.Marshal(s.data)
			version := s.Version()
			for i := 0; i < 3; i++ {
				got, err := s.registrarHistoricalReceipt(pinned, secret)
				if err != nil || got.Receipt != receipt || got.Digest != digest {
					t.Fatal("own historical receipt unavailable", err)
				}
				got.Receipt.NodeID = "untrusted mutation"
			}
			if action != "live" {
				if _, err := s.authorizeRegistrarCredential(ref, secret); err == nil {
					t.Fatal("history revived current access")
				}
			}
			after, _ := os.ReadFile(s.path)
			memoryAfter, _ := json.Marshal(s.data)
			if !bytes.Equal(before, after) || !bytes.Equal(memory, memoryAfter) || s.Version() != version {
				t.Fatal("history allocated/saved/notified/mutated")
			}
		})
	}
}

func TestRegistrarHistoricalReceiptEveryPinnedFieldAndSecretBound(t *testing.T) {
	s, _, secret := registrarCredentialFixture(t)
	c := registrarFixtureRecord(s).Consumption.Challenge
	for _, target := range []string{"challenge", "binding"} {
		v := reflect.ValueOf(c)
		if target == "binding" {
			v = reflect.ValueOf(c.Binding)
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).Kind() == reflect.Struct {
				continue
			}
			changed := c
			field := reflect.ValueOf(&changed).Elem()
			if target == "binding" {
				field = field.FieldByName("Binding")
			}
			field = field.Field(i)
			switch field.Kind() {
			case reflect.String:
				field.SetString(field.String() + "x")
			case reflect.Int64:
				field.SetInt(field.Int() + 1)
			case reflect.Int:
				field.SetInt(field.Int() + 1)
			default:
				t.Fatal("uncovered field")
			}
			if got, err := s.registrarHistoricalReceipt(changed, secret); !errors.Is(err, ErrUnauthorized) || got != (registrarReceiptHistory{}) {
				t.Fatal("changed pinned field returned history", target, v.Type().Field(i).Name, err)
			}
		}
	}
	for _, bad := range []string{"", secret + "=", registrarFixtureRecord(s).CredentialDigest, registrarContractDigest(43), registrarContractDigest(0)} {
		if got, err := s.registrarHistoricalReceipt(c, bad); !errors.Is(err, ErrUnauthorized) || got != (registrarReceiptHistory{}) {
			t.Fatal("wrong secret returned history", err)
		}
	}
	before, _ := os.ReadFile(s.path)
	version := s.Version()
	s.data.Registrar = nil
	if _, err := s.registrarHistoricalReceipt(c, secret); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("absence became success", err)
	}
	after, _ := os.ReadFile(s.path)
	if !bytes.Equal(before, after) || s.Version() != version {
		t.Fatal("absent attempt allocated or saved")
	}
}

func TestRegistrarHistoricalReceiptCorruptAndLostOwnerRefuse(t *testing.T) {
	for _, kind := range []string{"index", "receipt", "transcript", "mixed-token", "mixed-identity", "routes", "foreign-key", "legacy-v1", "closed", "frozen", "released-owner"} {
		t.Run(kind, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			r := registrarFixtureRecord(s)
			c := r.Consumption.Challenge
			switch kind {
			case "index":
				delete(s.data.Registrar.ConsumedProofs, registrarProofKey(c))
			case "receipt":
				r.ReceiptDigest = registrarContractDigest(44)
			case "transcript":
				r.Consumption.TranscriptDigest = registrarContractDigest(44)
			case "mixed-token":
				s.data.NodeTokens[r.NodeID] = secret
			case "mixed-identity":
				s.data.ApprovalIdentities = map[string]ApprovalIdentity{r.NodeID: {}}
			case "routes":
				s.data.Nodes[ref.NodeID].Routes = []string{"0.0.0.0/0"}
			case "foreign-key":
				s.data.Nodes[ref.NodeID].PubKey = "bad"
			case "legacy-v1":
				s.data.Registrar.Version = 1
			case "closed":
				s.Close()
			case "frozen":
				s.mu.Lock()
				s.freezeLocked(ErrStoreUncertain)
				s.mu.Unlock()
			case "released-owner":
				s.lease.owner.Close()
			}
			before, _ := json.Marshal(s.data)
			disk, _ := os.ReadFile(s.path)
			if got, err := s.registrarHistoricalReceipt(c, secret); err == nil || got != (registrarReceiptHistory{}) {
				t.Fatal("corrupt or lost owner returned history")
			}
			after, _ := json.Marshal(s.data)
			diskAfter, _ := os.ReadFile(s.path)
			if !bytes.Equal(before, after) || !bytes.Equal(disk, diskAfter) {
				t.Fatal("history repaired corruption")
			}
		})
	}
}

func TestRegistrarHistoricalReceiptConcurrentActualLifecycle(t *testing.T) {
	for _, action := range []string{"expire", "delete", "close"} {
		t.Run(action, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			r := registrarFixtureRecord(s)
			c := r.Consumption.Challenge
			digest := r.ReceiptDigest
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for n := 0; n < 20; n++ {
						got, err := s.registrarHistoricalReceipt(c, secret)
						if err == nil && got.Digest != digest {
							t.Error("history changed")
						}
						if action != "close" && err != nil {
							t.Error("retained history disappeared", err)
						}
					}
				}()
			}
			close(start)
			var err error
			switch action {
			case "expire":
				err = s.ExpireNode(ref.NodeID)
			case "delete":
				err = s.DeleteNode(ref.NodeID)
			case "close":
				err = s.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			got, err := s.registrarHistoricalReceipt(c, secret)
			if action == "close" {
				if err == nil {
					t.Fatal("closed history available")
				}
			} else if err != nil || got.Digest != digest {
				t.Fatal("historical lookup failed after transition", err)
			}
		})
	}
}

func TestRegistrarHistoricalReceiptActualPublicationFailureAndOwningReopen(t *testing.T) {
	for _, stage := range []string{"create-temp", "write", "short-write", "sync-file", "close-file", "rename", "rename-after-publish", "sync-directory"} {
		t.Run(stage, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			r := registrarFixtureRecord(s)
			c := r.Consumption.Challenge
			digest := r.ReceiptDigest
			real := s.fs
			s.fs.createTemp = func(dir string) (storeTemp, error) {
				if stage == "create-temp" {
					return nil, errInjectedStoreIO
				}
				f, err := real.createTemp(dir)
				if err != nil {
					return nil, err
				}
				return faultStoreTemp{f, stage}, nil
			}
			if stage == "rename" {
				s.fs.rename = func(string, string) error { return errInjectedStoreIO }
			}
			if stage == "rename-after-publish" {
				s.fs.rename = func(a, b string) error {
					if err := real.rename(a, b); err != nil {
						return err
					}
					return errInjectedStoreIO
				}
			}
			if stage == "sync-directory" {
				s.fs.syncDir = func(path string) error {
					if err := real.syncDir(path); err != nil {
						return err
					}
					return errInjectedStoreIO
				}
			}
			err := s.DeleteNode(ref.NodeID)
			if err == nil {
				t.Fatal("injected deletion succeeded")
			}
			uncertain := stage == "rename-after-publish" || stage == "sync-directory"
			before, _ := os.ReadFile(s.path)
			version := s.Version()
			got, lookupErr := s.registrarHistoricalReceipt(c, secret)
			if uncertain {
				if !errors.Is(err, ErrStoreUncertain) || lookupErr == nil || got != (registrarReceiptHistory{}) {
					t.Fatal("uncertain owner returned success history", err, lookupErr)
				}
			} else if lookupErr != nil || got.Digest != digest {
				t.Fatal("rollback-safe failure erased history", lookupErr)
			}
			after, _ := os.ReadFile(s.path)
			if !bytes.Equal(before, after) || s.Version() != version {
				t.Fatal("history changed publication outcome")
			}
			s = reopenRegistrarFixture(t, s)
			got, err = s.registrarHistoricalReceipt(c, secret)
			if err != nil || got.Digest != digest {
				t.Fatal("whole image history absent on owning reopen", err)
			}
			_, present := s.data.Nodes[ref.NodeID]
			if present == uncertain {
				t.Fatal("wrong whole-image deletion outcome")
			}
		})
	}
}
