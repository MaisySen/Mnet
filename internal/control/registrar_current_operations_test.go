package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fintex.au/mnet/internal/netmap"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestRegistrarCurrentOperationsActualReopenAndDetachedMap(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	version := s.Version()
	eps := []string{"192.0.2.2:51820", "192.0.2.1:51820", "192.0.2.2:51820", "invalid"}
	if err := s.setRegistrarCredentialEndpoints(ref, secret, eps); err != nil {
		t.Fatal(err)
	}
	if s.Version() <= version {
		t.Fatal("actual endpoint publication did not wake map version")
	}
	expected := []string{"192.0.2.1:51820", "192.0.2.2:51820"}
	got, err := s.registrarCredentialMap(ref, secret)
	if err != nil || !reflect.DeepEqual(got.Self.Endpoints, expected) || got.Self.ID != ref.NodeID {
		t.Fatal("credential map", err)
	}
	got.Self.Endpoints[0] = "mutated:1"
	got.Self.KeyExpiry = nil
	got.Network.Name = "mutated"
	again, err := s.registrarCredentialMap(ref, secret)
	if err != nil || !reflect.DeepEqual(again.Self.Endpoints, expected) || again.Self.KeyExpiry == nil || again.Network.Name == "mutated" {
		t.Fatal("map aliases owner", err)
	}
	if got.RelayURL != "" {
		t.Fatal("inactive map selected legacy credential-bearing relay")
	}
	before, _ := os.ReadFile(s.path)
	version = s.Version()
	if err := s.setRegistrarCredentialEndpoints(ref, secret, expected); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.path)
	if !bytes.Equal(before, after) || s.Version() != version {
		t.Fatal("same set rewrote or woke")
	}
	s = reopenRegistrarFixture(t, s)
	again, err = s.registrarCredentialMap(ref, secret)
	if err != nil || !reflect.DeepEqual(again.Self.Endpoints, expected) {
		t.Fatal("published endpoints absent after owning reopen", err)
	}
}
func TestRegistrarCurrentOperationsWrongSecretScopeNoEffect(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	for _, kind := range []string{"wrong-secret", "wrong-scope", "revoked", "deleted"} {
		t.Run(kind, func(t *testing.T) {
			testRef, testSecret := ref, secret
			switch kind {
			case "wrong-secret":
				testSecret = ref.ReceiptDigest
			case "wrong-scope":
				testRef.Scope.ServiceID += "x"
			case "revoked":
				if err := s.ExpireNode(ref.NodeID); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := s.DeleteNode(ref.NodeID); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := json.Marshal(s.data)
			disk, _ := os.ReadFile(s.path)
			version := s.Version()
			if m, err := s.registrarCredentialMap(testRef, testSecret); !errors.Is(err, ErrUnauthorized) || m != nil {
				t.Fatal("unauthorized map", err)
			}
			if err := s.setRegistrarCredentialEndpoints(testRef, testSecret, []string{"192.0.2.8:1"}); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("unauthorized endpoint update", err)
			}
			after, _ := json.Marshal(s.data)
			diskAfter, _ := os.ReadFile(s.path)
			if !bytes.Equal(before, after) || !bytes.Equal(disk, diskAfter) || s.Version() != version {
				t.Fatal("refusal mutated Store")
			}
		})
	}
}
func TestRegistrarEndpointActualPublicationRollbackFreezeAndReopen(t *testing.T) {
	for _, stage := range []string{"create-temp", "write", "short-write", "sync-file", "close-file", "rename", "rename-after-publish", "sync-directory"} {
		t.Run(stage, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			real := s.fs
			before, _ := json.Marshal(s.data)
			disk, _ := os.ReadFile(s.path)
			version := s.Version()
			s.fs.createTemp = func(dir string) (storeTemp, error) {
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
			eps := []string{"192.0.2.9:51820"}
			err := s.setRegistrarCredentialEndpoints(ref, secret, eps)
			if err == nil {
				t.Fatal("failed publication succeeded")
			}
			if s.Version() != version {
				t.Fatal("failed publication advanced version")
			}
			uncertain := stage == "rename-after-publish" || stage == "sync-directory"
			if uncertain {
				if !errors.Is(err, ErrStoreUncertain) {
					t.Fatal("missing uncertainty", err)
				}
				if m, err := s.registrarCredentialMap(ref, secret); err == nil || m != nil {
					t.Fatal("uncertain owner served credential map")
				}
				if err := s.setRegistrarCredentialEndpoints(ref, secret, eps); err == nil {
					t.Fatal("uncertain owner retried")
				}
			} else {
				after, _ := json.Marshal(s.data)
				diskAfter, _ := os.ReadFile(s.path)
				if !bytes.Equal(before, after) || !bytes.Equal(disk, diskAfter) {
					t.Fatal("rollback-safe failure changed state")
				}
				s.fs = real
				if err := s.setRegistrarCredentialEndpoints(ref, secret, eps); err != nil {
					t.Fatal("exact current retry", err)
				}
			}
			s = reopenRegistrarFixture(t, s)
			m, err := s.registrarCredentialMap(ref, secret)
			if err != nil || !reflect.DeepEqual(m.Self.Endpoints, eps) {
				t.Fatal("whole durable image not reopened", err)
			}
		})
	}
}
func TestRegistrarCurrentOperationsConcurrentRevocationLinearization(t *testing.T) {
	for _, operation := range []string{"expire", "delete", "close"} {
		t.Run(operation, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			start := make(chan struct{})
			stop := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for {
						select {
						case <-stop:
							return
						default:
						}
						s.registrarCredentialMap(ref, secret)
						s.setRegistrarCredentialEndpoints(ref, secret, []string{"192.0.2.1:51820"})
					}
				}()
			}
			close(start)
			switch operation {
			case "expire":
				if err := s.ExpireNode(ref.NodeID); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := s.DeleteNode(ref.NodeID); err != nil {
					t.Fatal(err)
				}
			case "close":
				s.Close()
			}
			for i := 0; i < 30; i++ {
				if m, err := s.registrarCredentialMap(ref, secret); err == nil || m != nil {
					t.Fatal("post-revocation credential map")
				}
				if err := s.setRegistrarCredentialEndpoints(ref, secret, []string{"192.0.2.2:1"}); err == nil {
					t.Fatal("post-revocation endpoints")
				}
			}
			close(stop)
			wg.Wait()
		})
	}
}

func TestRegistrarCurrentMapUsesActualLegacyACLAndIsolation(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	join := func(network, name string, b byte) *netmap.JoinResponse {
		t.Helper()
		n, err := s.Join(netmap.JoinRequest{Network: network, Name: name, PubKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32))})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ApproveNode(n.NodeID); err != nil {
			t.Fatal(err)
		}
		return n
	}
	join(ref.NetworkID, "allowed-peer", 121)
	join(ref.NetworkID, "denied-peer", 122)
	other, err := s.CreateNetwork("other")
	if err != nil {
		t.Fatal(err)
	}
	join(other.ID, "isolated-peer", 123)
	s.mu.Lock()
	s.data.Networks[ref.NetworkID].ACL = &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "fixture", Dst: "allowed-peer"}}}
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	m, err := s.registrarCredentialMap(ref, secret)
	if err != nil || len(m.Peers) != 1 || m.Peers[0].Name != "allowed-peer" {
		t.Fatal("registrar path bypassed ACL/network isolation", err)
	}
	m.Network.ACL.Rules[0].Dst = "mutated"
	m.Peers[0].Name = "mutated"
	again, err := s.registrarCredentialMap(ref, secret)
	if err != nil || again.Network.ACL.Rules[0].Dst != "allowed-peer" || again.Peers[0].Name != "allowed-peer" {
		t.Fatal("ACL/peer snapshot aliased owner", err)
	}
}
