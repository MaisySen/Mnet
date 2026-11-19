//go:build linux

package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

func TestDeskRunReservationAndPrivateFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	var successes atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if reserveDeskRun(dir, token) == nil {
				successes.Add(1)
			}
		}()
	}
	group.Wait()
	if successes.Load() != 1 {
		t.Fatal("reuse", successes.Load())
	}
	if reserveDeskRun(dir, "../x") == nil {
		t.Fatal("path traversal")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(dir, link)
	if reserveDeskRun(link, strings.Repeat("b", 64)) == nil {
		t.Fatal("symlink directory")
	}
	os.Chmod(dir, 0755)
	if reserveDeskRun(dir, strings.Repeat("b", 64)) == nil {
		t.Fatal("nonprivate state")
	}
	key := filepath.Join(t.TempDir(), "key")
	os.WriteFile(key, []byte("local test bytes"), 0600)
	if _, err := readDeskLocalFile(key, true, 64); err != nil {
		t.Fatal(err)
	}
	if _, err := readDeskLocalFile(key, true, 2); err == nil {
		t.Fatal("oversized")
	}
	os.Chmod(key, 0644)
	if _, err := readDeskLocalFile(key, true, 64); err == nil {
		t.Fatal("nonprivate key")
	}
	os.Symlink(key, key+"-link")
	if _, err := readDeskLocalFile(key+"-link", false, 64); err == nil {
		t.Fatal("symlink key")
	}
}
func TestDeskContextCloserJoined(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a, b := net.Pipe()
	defer b.Close()
	join := closeOnDeskContext(ctx, a)
	cancel()
	join()
	if _, err := a.Write([]byte{0}); err == nil {
		t.Fatal("connection not closed")
	}
}

// Write disposable public/private material without logging it. This helper
// enables an external driver to launch the actual daemon, not a test-only path.
func TestDeskX11PrepareLauncher(t *testing.T) {
	if os.Getenv("MNET_DESK_LAUNCH_PREPARE") != "1" {
		t.Skip("disposable actual-launch fixture only")
	}
	dir := os.Getenv("MNET_DESK_ARTIFACTS")
	targetDisplay := os.Getenv("MNET_DESK_TARGET_DISPLAY")
	source, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := xgb.NewConnDisplay(targetDisplay)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	keyboard, err := deskKeyboardFingerprint(source)
	if err != nil {
		t.Fatal(err)
	}
	targetMap, err := deskKeyboardFingerprint(target)
	if err != nil || targetMap != keyboard {
		t.Fatal("map mismatch")
	}
	a, ak := deskCertificateFixture(t)
	b, bk := deskCertificateFixture(t)
	ss, ts := xproto.Setup(source).DefaultScreen(source), xproto.Setup(target).DefaultScreen(target)
	l := desk.Layout{Version: 1, Displays: []desk.Display{{ID: "source", Device: ak, Width: 1000, Height: 1000, PixelWidth: int64(ss.WidthInPixels), PixelHeight: int64(ss.HeightInPixels)}, {ID: "target", Device: bk, X: 1000, Width: 1000, Height: 1000, PixelWidth: int64(ts.WidthInPixels), PixelHeight: int64(ts.HeightInPixels)}}}
	compiled, err := desk.Compile(l)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	spec := desk.RunSpec{Version: 1, Address: address, RunDirectory: "runs", LifetimeSeconds: 30, SourceDisplay: "source", Edge: desk.Right, Binding: desk.Binding{Run: strings.Repeat("e", 64), Primary: ak, Recipient: bk, Layout: compiled.Fingerprint(), Keyboard: keyboard, Display: "target"}, Layout: l}
	for _, role := range []string{"primary", "recipient"} {
		cert := a
		if role == "recipient" {
			cert = b
		}
		path := filepath.Join(dir, role)
		if err = os.MkdirAll(filepath.Join(path, "runs"), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(path, "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600); err != nil {
			t.Fatal(err)
		}
		private, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(path, "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); err != nil {
			t.Fatal(err)
		}
		spec.Role = role
		spec.Certificate = "cert.pem"
		spec.PrivateKey = "key.pem"
		data, _ := json.MarshalIndent(spec, "", "  ")
		if err = os.WriteFile(filepath.Join(path, "run.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadDeskCertificate(specWithPaths(spec, path))
		if err != nil || loaded.Leaf == nil {
			t.Fatal(err)
		}
	}
}
func specWithPaths(s desk.RunSpec, path string) desk.RunSpec {
	s.Certificate = filepath.Join(path, s.Certificate)
	s.PrivateKey = filepath.Join(path, s.PrivateKey)
	return s
}
func TestDeskSafetyHandoffRetainsActualStop(t *testing.T) {
	// Deterministic event disposition regression, native stop/ownership cases
	// separately run on disposable X servers.
	s := &x11AttendedSafety{done: make(chan struct{})}
	s.once.Do(func() {})
	close(s.done)
	s.interrupted.Store(true)
	if s.closeForDeskHandoff() == nil {
		t.Fatal("deliberate close hid actual interruption")
	}
	s.interrupted.Store(false)
	if err := s.closeForDeskHandoff(); err != nil {
		t.Fatal(err)
	}
}

func TestDeskX11SafetyStopSurvivesHandoffRace(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("disposable X11 only")
	}
	c, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err = xtest.Init(c); err != nil {
		t.Fatal(err)
	}
	keyboard, err := deskKeyboardFingerprint(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newDeskSafety(keyboard)
	if err != nil {
		t.Fatal(err)
	}
	root := xproto.Setup(c).DefaultScreen(c).Root
	codes := []byte{keycodeFor(c, 0xffe3), keycodeFor(c, 0xffe9), keycodeFor(c, 0xffe1), attendedStopKey(c)}
	for _, code := range codes {
		if code == 0 {
			t.Fatal("fixture stop key unavailable")
		}
		if err = xtest.FakeInputChecked(c, xproto.KeyPress, code, 0, root, 0, 0, 0).Check(); err != nil {
			t.Fatal(err)
		}
	}
	c.Sync()
	// Deliberately do not wait on Stopped. The transfer must join pending actual
	// local stop processing, not turn its own close into an apparent clean grant.
	err = closeDeskSafetyForHandoff(s)
	for i := len(codes) - 1; i >= 0; i-- {
		xtest.FakeInputChecked(c, xproto.KeyRelease, codes[i], 0, root, 0, 0, 0).Check()
	}
	c.Sync()
	if err == nil {
		t.Fatal("actual shortcut erased by handoff")
	}
}
