package daemon

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

var errNativeInjected = errors.New("injected native persistence failure")

type nativeFaultTemp struct {
	stateTemp
	stage string
}

func (f *nativeFaultTemp) Write(b []byte) (int, error) {
	if f.stage == "write" {
		return 0, errNativeInjected
	}
	if f.stage == "short" {
		return f.stateTemp.Write(b[:len(b)/2])
	}
	return f.stateTemp.Write(b)
}
func (f *nativeFaultTemp) Sync() error {
	if f.stage == "sync" {
		return errNativeInjected
	}
	return f.stateTemp.Sync()
}
func (f *nativeFaultTemp) Close() error {
	err := f.stateTemp.Close()
	if f.stage == "close" {
		return errNativeInjected
	}
	return err
}
func nativeOwned(t *testing.T) (*Daemon, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	if _, err := d.PrepareIdentity(); err != nil {
		t.Fatal(err)
	}
	return d, dir
}
func TestNativeOwnerRealFilePrepublicationRollbackAndExactRetry(t *testing.T) {
	for _, stage := range []string{"create", "write", "short", "sync", "close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			d, dir := nativeOwned(t)
			original := d.stateOwner.fs
			before := d.State()
			diskBefore, err := os.ReadFile(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			d.stateOwner.fs.create = func(dir string) (stateTemp, error) {
				if stage == "create" {
					return nil, errNativeInjected
				}
				f, err := original.create(dir)
				if err != nil {
					return nil, err
				}
				return &nativeFaultTemp{f, stage}, nil
			}
			if stage == "rename" {
				d.stateOwner.fs.rename = func(string, string) error { return errNativeInjected }
			}
			if err := d.SetRouting(true, []string{"10.20.0.0/24"}, "candidate"); err == nil {
				t.Fatal("failure reported success")
			}
			diskAfter, err := os.ReadFile(statePath(dir))
			if err != nil || !bytes.Equal(diskBefore, diskAfter) {
				t.Fatal("prepublish changed actual file", err)
			}
			after := d.State()
			if after.PublicKey != before.PublicKey || after.PrivateKey != before.PrivateKey || after.ExitNode || after.UseExitNode != "" || d.stateOwner.fault {
				t.Fatal("prepublish changed working identity/config")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 2 {
				t.Fatal("temporary native file leaked", err)
			}
			d.stateOwner.fs = original
			if err := d.SetRouting(true, []string{"10.20.0.0/24"}, "candidate"); err != nil {
				t.Fatal("exact retry failed", err)
			}
			d.Close()
			saved, err := LoadState(dir)
			if err != nil || !saved.ExitNode || saved.PublicKey != before.PublicKey || saved.PrivateKey != before.PrivateKey {
				t.Fatal("retry not durable", err)
			}
		})
	}
}
func TestNativeOwnerRealUncertainPublicationFreezesAndReopensWholeImage(t *testing.T) {
	for _, stage := range []string{"after-rename", "dirsync"} {
		t.Run(stage, func(t *testing.T) {
			d, dir := nativeOwned(t)
			before := d.State()
			original := d.stateOwner.fs
			if stage == "after-rename" {
				d.stateOwner.fs.rename = func(a, b string) error {
					if err := original.rename(a, b); err != nil {
						return err
					}
					return errNativeInjected
				}
			} else {
				d.stateOwner.fs.syncDir = func(string) error { return errNativeInjected }
			}
			if err := d.SetRouting(true, []string{"10.25.0.0/24"}, "gateway"); !errors.Is(err, ErrStateUncertain) {
				t.Fatal("ambiguous write not frozen", err)
			}
			if _, err := d.PrepareIdentity(); err == nil {
				t.Fatal("uncertain owner returned usable identity")
			}
			if err := d.SetRouting(false, nil, ""); err == nil {
				t.Fatal("uncertain owner mutated")
			}
			if err := d.Up("https://must-not-contact.invalid", "network", "", ""); err == nil {
				t.Fatal("uncertain owner admitted network startup")
			}
			if d.currentMap() != nil || d.client() != nil || len(d.SessionPeers()) != 0 || d.State().PrivateKey != "" {
				t.Fatal("uncertain state exposed authority/secret")
			}
			d.Close()
			fresh, err := New(Config{StateDir: dir})
			if err != nil {
				t.Fatal("whole image reopen failed", err)
			}
			defer fresh.Close()
			saved := fresh.State()
			if !saved.ExitNode || saved.UseExitNode != "gateway" || saved.PrivateKey != before.PrivateKey || saved.PublicKey != before.PublicKey {
				t.Fatal("reopen lost candidate or changed identity")
			}
		})
	}
}
func TestNativeOwnerGenerationPublicationFaultHasNoUnpublishedSuccess(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	original := d.stateOwner.fs
	d.stateOwner.fs.rename = func(string, string) error { return errNativeInjected }
	if key, err := d.PrepareIdentity(); err == nil || key != "" || d.State().PrivateKey != "" {
		t.Fatal("failed generation returned unpublished key")
	}
	d.stateOwner.fs = original
	if key, err := d.PrepareIdentity(); err != nil || key == "" {
		t.Fatal("generation retry failed", err)
	}
}
func TestNativeOwnerCopiedDetachedSnapshotDoesNotShareRevision(t *testing.T) {
	dir := t.TempDir()
	s := State{NodeName: "original"}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	copied := s
	s.NodeName = "winner"
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	copied.NodeName = "loser"
	if err := copied.Save(dir); !errors.Is(err, ErrStateStale) {
		t.Fatal("copied snapshot revision advanced by reference", err)
	}
	other := t.TempDir()
	if err := s.Save(other); !errors.Is(err, ErrStateStale) {
		t.Fatal("cross-directory snapshot copied identity", err)
	}
}
func TestNativeOwnerAliasHardlinkAndUnsafeShapeRefused(t *testing.T) {
	d, dir := nativeOwned(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err == nil {
		if second, err := New(Config{StateDir: alias}); err == nil {
			second.Close()
			t.Fatal("alias admitted second owner")
		}
	}
	d.Close()
	for _, kind := range []string{"hardlink", "duplicate", "case", "null", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			path := statePath(dir)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			defer os.WriteFile(path, before, 0600)
			var raw []byte
			switch kind {
			case "hardlink":
				link := filepath.Join(dir, "state-alias")
				if err := os.Link(path, link); err != nil {
					t.Skip(err)
				}
				defer os.Remove(link)
				raw = before
			case "duplicate":
				raw = []byte(`{"node_name":"a","node_name":"b"}`)
			case "case":
				raw = []byte(`{"Node_Name":"alias"}`)
			case "null":
				raw = []byte(`{"private_key":null}`)
			case "unknown":
				raw = []byte(`{"future_credential":"not-a-default"}`)
			}
			if kind != "hardlink" {
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if fresh, err := New(Config{StateDir: dir}); err == nil {
				fresh.Close()
				t.Fatal("unsafe native state accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(raw, after) {
				t.Fatal("unsafe state content changed", err)
			}
		})
	}
}
func TestNativeOwnerReplacementPermanentlyFreezes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("held lock rename denied natively; native OS execution required")
	}
	d, dir := nativeOwned(t)
	lock := filepath.Join(dir, "daemon.lock")
	if err := os.Rename(lock, lock+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PrepareIdentity(); err == nil {
		t.Fatal("lost lock not detected")
	}
	os.Remove(lock)
	if err := os.Rename(lock+".old", lock); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PrepareIdentity(); err == nil {
		t.Fatal("restoring path resurrected identity")
	}
}
func TestNativeOwnerConcurrentCloseAndMutation(t *testing.T) {
	d, dir := nativeOwned(t)
	before := d.State()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				d.Close()
			} else {
				d.SetRouting(i%2 == 0, nil, "")
			}
		}(i)
	}
	wg.Wait()
	fresh, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if fresh.State().PublicKey != before.PublicKey {
		t.Fatal("Close race changed identity")
	}
}
func TestNativeOwnerSubprocessDeathReleasesSameDaemonLock(t *testing.T) {
	if dir := os.Getenv("MNET_NATIVE_OWNER_TEST_CHILD"); dir != "" {
		d, err := New(Config{StateDir: dir})
		if err != nil {
			os.Exit(4)
		}
		defer d.Close()
		os.Stdout.WriteString("native-ready\n")
		time.Sleep(time.Minute)
		return
	}
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestNativeOwnerSubprocessDeathReleasesSameDaemonLock$")
	cmd.Env = append(os.Environ(), "MNET_NATIVE_OWNER_TEST_CHILD="+dir)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(pipe); ready <- scanner.Scan() && scanner.Text() == "native-ready" }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child failed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child timeout")
	}
	if d, err := New(Config{StateDir: dir}); err == nil {
		d.Close()
		t.Fatal("second process admitted")
	}
	cmd.Process.Kill()
	cmd.Wait()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal("process death retained lock", err)
	}
	d.Close()
}

var _ io.Writer = (*nativeFaultTemp)(nil)
