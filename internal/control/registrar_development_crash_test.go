//go:build (mnet_enrollment_development || multies_native_fixture) && !windows

package control

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type developmentKillTemp struct {
	storeTemp
	stage  string
	signal func()
}

func (f developmentKillTemp) Write(raw []byte) (int, error) {
	n, e := f.storeTemp.Write(raw)
	if f.stage == "after-write" {
		f.signal()
	}
	return n, e
}
func (f developmentKillTemp) Sync() error {
	e := f.storeTemp.Sync()
	if f.stage == "after-filesync" {
		f.signal()
	}
	return e
}

// Child pauses at the named actual I/O boundary. Parent sends real SIGKILL,
// not an injected returned error, panic or clean Close. Kernel/power loss is
// NOT simulated and post-rename directory durability cannot be proven by kill.
func TestDevelopmentStoreActualSIGKILLChild(t *testing.T) {
	path := os.Getenv("MNET_TEST_DISPOSABLE_KILL_PATH")
	if path == "" {
		t.Skip("child only")
	}
	stage := os.Getenv("MNET_TEST_DISPOSABLE_KILL_STAGE")
	s, e := openStore(path, developmentRegistrarCodec{})
	if e != nil {
		t.Fatal(e)
	}
	signal := func() {
		if e := os.WriteFile(path+".at-boundary", []byte(stage), 0600); e != nil {
			t.Fatal(e)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	real := s.fs
	s.fs.createTemp = func(dir string) (storeTemp, error) {
		tmp, e := real.createTemp(dir)
		if e != nil {
			return nil, e
		}
		if stage == "after-tempcreate" {
			signal()
		}
		return developmentKillTemp{tmp, stage, signal}, nil
	}
	s.fs.rename = func(a, b string) error {
		if stage == "before-rename" {
			signal()
		}
		e := real.rename(a, b)
		if stage == "after-rename" {
			signal()
		}
		return e
	}
	s.fs.syncDir = func(p string) error {
		e := real.syncDir(p)
		if stage == "after-dirsync" {
			signal()
		}
		return e
	}
	if _, e = s.CreateNetwork("after"); e != nil {
		t.Fatal(e)
	}
	t.Fatal("child missed kill boundary")
}
func TestDevelopmentStoreActualSIGKILLWholeImages(t *testing.T) {
	for _, stage := range []string{"after-tempcreate", "after-write", "after-filesync", "before-rename", "after-rename", "after-dirsync"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "image")
			s, e := Open(path)
			if e != nil {
				t.Fatal(e)
			}
			s.storageCodec = developmentRegistrarCodec{}
			if _, e = s.CreateNetwork("before"); e != nil {
				t.Fatal(e)
			}
			s.Close()
			before, _ := os.ReadFile(path)
			cmd := exec.Command(os.Args[0], "-test.run=^TestDevelopmentStoreActualSIGKILLChild$", "-test.timeout=30s")
			cmd.Env = append(os.Environ(), "MNET_TEST_DISPOSABLE_KILL_PATH="+path, "MNET_TEST_DISPOSABLE_KILL_STAGE="+stage)
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, e := os.Stat(path + ".at-boundary"); e == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("child boundary timeout")
				}
				time.Sleep(2 * time.Millisecond)
			}
			if e = cmd.Process.Signal(syscall.SIGKILL); e != nil {
				t.Fatal(e)
			}
			e = cmd.Wait()
			if e == nil {
				t.Fatal("SIGKILL child returned success")
			}
			status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatalf("not actual SIGKILL %v %s", e, output.String())
			}
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			payload, e := (developmentRegistrarCodec{}).decode(raw)
			if e != nil {
				t.Fatal(e)
			}
			var data storeData
			if json.Unmarshal(payload, &data) != nil {
				t.Fatal("torn JSON")
			}
			post := stage == "after-rename" || stage == "after-dirsync"
			if !post && (!bytes.Equal(raw, before) || len(data.Networks) != 1) {
				t.Fatal("pre-rename kill changed image")
			}
			if post && len(data.Networks) != 2 {
				t.Fatal("post-rename complete image absent")
			}
			reopened, e := openStore(path, developmentRegistrarCodec{})
			if e != nil {
				t.Fatal(e)
			}
			reopened.Close()
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatal("recovery rewrote image")
			}
			t.Logf("actual SIGKILL boundary=%s completeNetworks=%d noRewrite=true", stage, len(data.Networks))
		})
	}
}
