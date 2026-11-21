package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

var errInjectedStoreIO = errors.New("injected Store I/O failure")

type faultStoreTemp struct {
	storeTemp
	stage string
}

func (f faultStoreTemp) Write(b []byte) (int, error) {
	if f.stage == "write" {
		return 0, errInjectedStoreIO
	}
	if f.stage == "short-write" {
		return f.storeTemp.Write(b[:len(b)-1])
	}
	return f.storeTemp.Write(b)
}
func (f faultStoreTemp) Sync() error {
	if f.stage == "sync-file" {
		return errInjectedStoreIO
	}
	return f.storeTemp.Sync()
}
func (f faultStoreTemp) Close() error {
	err := f.storeTemp.Close()
	if f.stage == "close-file" {
		return errInjectedStoreIO
	}
	return err
}

type lifecycleFixture struct {
	s       *Store
	network *netmap.Network
	node    *netmap.JoinResponse
	peer    *netmap.JoinResponse
	key     *netmap.AuthKey
}

func newLifecycleFixture(t *testing.T) lifecycleFixture {
	t.Helper()
	s, err := openTestStore(t, filepath.Join(t.TempDir(), "nested", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := s.CreateNetwork("fixture")
	if err != nil {
		t.Fatal(err)
	}
	join := func(b byte) *netmap.JoinResponse {
		raw := make([]byte, 32)
		raw[0] = b
		node, err := s.Join(netmap.JoinRequest{Network: network.ID, Name: "fixture", PubKey: base64.StdEncoding.EncodeToString(raw)})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ApproveNode(node.NodeID); err != nil {
			t.Fatal(err)
		}
		return node
	}
	node, peer := join(1), join(2)
	if err = s.ApproveNodeIdentity(node.NodeID, "display", "https://issuer.invalid", "subject"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetNetworkMember(network.ID, "https://issuer.invalid", "subject", true); err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAuthKey(network.ID, "fixture", false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return lifecycleFixture{s, network, node, peer, key}
}

func assertLifecycleAuthorityDenied(t *testing.T, f lifecycleFixture) {
	t.Helper()
	s := f.s
	if s.Health() == nil {
		t.Error("unhealthy Store reports healthy")
	}
	if _, err := s.AuthorizeNode(f.node.NodeID, f.node.NodeToken); err == nil {
		t.Error("unhealthy credential authorized")
	}
	if _, err := s.JoinStatus(f.node.NodeID, f.node.NodeToken); err == nil {
		t.Error("unhealthy status authorized")
	}
	if _, err := s.Map(f.node.NodeID); err == nil {
		t.Error("unhealthy map served")
	}
	peerKey := s.data.Nodes[f.peer.NodeID].PubKey
	if s.RelayAllowed(f.node.NodeID, peerKey) {
		t.Error("unhealthy relay allowed")
	}
	if _, err := s.IdentityDevices(f.network.ID, "https://issuer.invalid", "subject"); err == nil {
		t.Error("unhealthy account devices served")
	}
	if _, err := s.ApprovedDevicesForIdentity(f.network.ID, "https://issuer.invalid", "subject"); err == nil {
		t.Error("unhealthy alternate devices served")
	}
	if _, err := s.OperatorMemberships(f.network.ID); err == nil {
		t.Error("unhealthy memberships served")
	}
	if err := s.SetNetworkMember(f.network.ID, "https://issuer.invalid", "subject", true); err == nil {
		t.Error("unhealthy existing-grant no-op succeeded")
	}
	if err := s.SetNetworkMember(f.network.ID, "https://issuer.invalid", "absent", false); err == nil {
		t.Error("unhealthy absent-revoke no-op succeeded")
	}
	if _, err := s.CreateNetwork("denied"); err == nil {
		t.Error("unhealthy mutation succeeded")
	}
	if _, err := s.CreateAuthKey(f.network.ID, "denied", true, time.Hour); err == nil {
		t.Error("unhealthy key created")
	}
	if err := s.ApproveNode(f.node.NodeID); err == nil {
		t.Error("unhealthy approval succeeded")
	}
	if s.Network(f.network.ID) != nil || len(s.Networks()) != 0 || len(s.Snapshot().Nodes) != 0 {
		t.Error("unhealthy alternate snapshot exposed")
	}
	before := s.data.Nodes[f.node.NodeID].LastSeen
	s.Touch(f.node.NodeID)
	if !before.Equal(s.data.Nodes[f.node.NodeID].LastSeen) {
		t.Error("unhealthy Touch mutated state")
	}
	app, _, err := NewServer(s, nil, "https://control.invalid", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/healthz", "/api/map", "/account/api/devices", "/admin/api/state", "/api/relay/ws"} {
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("unhealthy HTTP %s returned %d", path, rec.Code)
		}
	}
}

func TestStorePublicationFailureMatrix(t *testing.T) {
	for _, stage := range []string{"create-temp", "write", "short-write", "sync-file", "close-file", "rename", "rename-after-publish", "sync-directory"} {
		t.Run(stage, func(t *testing.T) {
			f := newLifecycleFixture(t)
			s := f.s
			before, _ := json.Marshal(s.data)
			diskBefore, err := os.ReadFile(s.path)
			if err != nil {
				t.Fatal(err)
			}
			version := s.Version()
			watcher := make(chan struct{})
			s.watchers[f.network.ID] = map[chan struct{}]bool{watcher: true}
			real := s.fs
			s.fs.createTemp = func(dir string) (storeTemp, error) {
				if stage == "create-temp" {
					return nil, errInjectedStoreIO
				}
				tmp, err := real.createTemp(dir)
				if err != nil {
					return nil, err
				}
				return faultStoreTemp{tmp, stage}, nil
			}
			if stage == "rename" {
				s.fs.rename = func(string, string) error { return errInjectedStoreIO }
			}
			if stage == "rename-after-publish" {
				s.fs.rename = func(from, to string) error {
					if err := real.rename(from, to); err != nil {
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
			raw := make([]byte, 32)
			raw[0] = 3
			result, err := s.Join(netmap.JoinRequest{Network: f.network.ID, Name: "candidate", PubKey: base64.StdEncoding.EncodeToString(raw), AuthKey: f.key.Key})
			if err == nil || result != nil {
				t.Fatal("failed persistence returned enrollment success")
			}
			if s.Version() != version {
				t.Fatal("failed persistence published a map version")
			}
			diskAfter, readErr := os.ReadFile(s.path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if stage == "sync-directory" || stage == "rename-after-publish" {
				if !errors.Is(err, ErrStoreUncertain) {
					t.Fatalf("missing typed uncertainty: %v", err)
				}
				select {
				case <-watcher:
				default:
					t.Fatal("uncertain publication did not wake map waiter")
				}
				if bytes.Equal(diskBefore, diskAfter) || s.data.AuthKeys[f.key.ID].Uses != 1 {
					t.Fatal("postrename candidate incorrectly rolled back")
				}
				assertLifecycleAuthorityDenied(t, f)
			} else {
				after, _ := json.Marshal(s.data)
				if !bytes.Equal(before, after) || !bytes.Equal(diskBefore, diskAfter) {
					t.Fatal("prepublication failure changed live/disk authority")
				}
				if err := s.Health(); err != nil {
					t.Fatal("rollback-safe failure froze Store", err)
				}
				select {
				case <-watcher:
					t.Fatal("rollback-safe failure woke map waiter")
				default:
				}
			}
			matches, err := filepath.Glob(filepath.Join(filepath.Dir(s.path), ".mnet-state-*"))
			if err != nil || len(matches) != 0 {
				t.Fatal("temporary state leaked", matches, err)
			}
			path := s.path
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := openTestStore(t, path)
			if err != nil {
				t.Fatal(err)
			}
			expectedUses := 0
			if stage == "sync-directory" || stage == "rename-after-publish" {
				expectedUses = 1
			}
			if reopened.data.AuthKeys[f.key.ID].Uses != expectedUses || len(reopened.data.Nodes) != 2+expectedUses {
				t.Fatal("reopen did not recover one complete image")
			}
		})
	}
}

func TestStoreClosedAuthorityAndWaiterShutdown(t *testing.T) {
	f := newLifecycleFixture(t)
	s := f.s
	version := s.Version()
	done := make(chan int64, 1)
	go func() { done <- s.WaitForChangeContext(context.Background(), f.network.ID, version, time.Minute) }()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		n := len(s.watchers[f.network.ID])
		s.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiter did not register")
		}
		time.Sleep(time.Millisecond)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got != version {
			t.Fatal("shutdown claimed a map version")
		}
	case <-time.After(time.Second):
		t.Fatal("Close stranded map waiter")
	}
	assertLifecycleAuthorityDenied(t, f)
}

func TestStoreLockReplacementFreezesAuthority(t *testing.T) {
	f := newLifecycleFixture(t)
	if err := os.Rename(f.s.lease.path, f.s.lease.path+".moved"); err != nil {
		t.Skipf("OS prevents replacing open ownership file: %v", err)
	}
	if err := os.WriteFile(f.s.lease.path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	assertLifecycleAuthorityDenied(t, f)
	if err := os.Remove(f.s.lease.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.s.lease.path+".moved", f.s.lease.path); err != nil {
		t.Fatal(err)
	}
	if f.s.Health() == nil {
		t.Fatal("restoring lock path revived frozen owner")
	}
}
