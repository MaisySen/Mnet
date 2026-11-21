//go:build linux || darwin || windows || freebsd || openbsd || netbsd || dragonfly

package control

// Owning lifecycle contract for the real Store. The original opt-in failing
// baseline is retained in review evidence. No enrollment consumer is connected.
import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type lifecycleCloser interface{ Close() error }

func closeContractStore(t *testing.T, s *Store) {
	t.Helper()
	if c, ok := any(s).(lifecycleCloser); ok {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}
}

func TestRegistrarLifecycleRejectSecondWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeContractStore(t, s)
	// Ownership must start before the first state write, not after rename.
	second, err := Open(path)
	if second != nil {
		closeContractStore(t, second)
	}
	if err == nil || second != nil {
		t.Fatal("Open admitted a second writable Store before first write")
	}
}

func TestRegistrarLifecycleParentAliasCannotAcquireSecondWriter(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "real")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Skipf("parent symlink unavailable: %v", err)
	}
	s, err := Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeContractStore(t, s)
	second, err := Open(filepath.Join(alias, "state.json"))
	if second != nil {
		closeContractStore(t, second)
	}
	if err == nil || second != nil {
		t.Fatal("parent alias bypassed Store ownership")
	}
}

func TestRegistrarLifecycleRejectHardlinkedState(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "state.json")
	if err := os.WriteFile(original, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias.json")
	if err := os.Link(original, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{original, alias} {
		s, err := Open(path)
		if s != nil {
			closeContractStore(t, s)
		}
		if err == nil || s != nil {
			t.Errorf("hardlinked state admitted as writable: %s", filepath.Base(path))
		}
	}
}

func TestRegistrarLifecycleCloseEndsAuthorityAndReopenRefreshes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeContractStore(t, s)
	if _, err := s.CreateNetwork("before-close"); err != nil {
		t.Fatal(err)
	}
	closer, ok := any(s).(lifecycleCloser)
	if !ok {
		t.Fatal("Store lacks an ownership-release lifecycle")
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal("Close must be idempotent", err)
	}
	if _, err := s.CreateNetwork("closed-writer"); err == nil {
		t.Fatal("closed Store retained write authority")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeContractStore(t, reopened)
	if reopened.Network("before-close") == nil || reopened.Network("closed-writer") != nil {
		t.Fatal("reopen did not recover the last committed image")
	}
}

func TestRegistrarLifecycleSubprocessWriter(t *testing.T) {
	if path := os.Getenv("MNET_CONTRACT_SECOND_WRITER_PATH"); path != "" {
		s, err := Open(path)
		if s != nil {
			closeContractStore(t, s)
		}
		if err == nil || s != nil {
			t.Fatal("independent process acquired an owned Store")
		}
		return
	}
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeContractStore(t, s)
	if _, err := s.CreateNetwork("owned"); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestRegistrarLifecycleSubprocessWriter$", "-test.count=1")
	child.Env = append(os.Environ(), "MNET_CONTRACT_SECOND_WRITER_PATH="+path)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("cross-process ownership failed: %v\n%s", err, output)
	}
}

// A real child owns the Store until killed. No PID-file stale-owner guessing.
func TestRegistrarLifecycleProcessDeathReleasesOwner(t *testing.T) {
	if path := os.Getenv("MNET_CONTRACT_CRASH_OWNER_PATH"); path != "" {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateNetwork("crash-owner"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stdout.WriteString("STORE_READY\n"); err != nil {
			t.Fatal(err)
		}
		// Blocking pipe survives until the parent kills this process.
		var b [1]byte
		os.Stdin.Read(b[:])
		return
	}
	path := filepath.Join(t.TempDir(), "state.json")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRegistrarLifecycleProcessDeathReleasesOwner$", "-test.count=1")
	child.Env = append(os.Environ(), "MNET_CONTRACT_CRASH_OWNER_PATH="+path)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "STORE_READY\n" {
		t.Fatal("child did not acquire Store", ready, err)
	}
	if second, err := Open(path); err == nil || second != nil {
		if second != nil {
			second.Close()
		}
		t.Fatal("live child ownership bypassed")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("child was expected to be killed")
	}
	recovered, err := Open(path)
	if err != nil {
		t.Fatal("crash left stale ownership", err)
	}
	defer recovered.Close()
	if recovered.Network("crash-owner") == nil {
		t.Fatal("crash recovery lost committed network")
	}
}
