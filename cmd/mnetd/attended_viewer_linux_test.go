//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/localapi"
	"fintex.au/mnet/internal/session"
)

// Actual native host approval/capture and the built CLI viewer, on separate
// owned X displays. Local API identity evidence is an explicit fixture, not
// proof of a live workspace/Multies integration or physical computers.
func TestAttendedX11BuiltViewOnlyClient(t *testing.T) {
	if os.Getenv("MNET_NATIVE_VIEWER_TEST") != "1" {
		t.Skip("requires two disposable X displays and built viewer")
	}
	dir := os.Getenv("MNET_ATTENDED_ARTIFACTS")
	viewerBin := os.Getenv("MNET_VIEWER_EXECUTABLE")
	viewerDisplay := os.Getenv("MNET_VIEWER_DISPLAY")
	if dir == "" || viewerBin == "" || viewerDisplay == "" {
		t.Fatal("explicit fixture paths/displays required")
	}
	mode := os.Getenv("MNET_VIEWER_CASE")
	if mode == "" {
		mode = "local-end"
	}
	switch mode {
	case "local-end", "pending-close", "decline", "host-end", "authority-loss", "display-loss":
	default:
		t.Fatal("unsupported fixture case")
	}
	current := func() bool { _, err := os.Stat(filepath.Join(dir, "withdraw-authority")); return os.IsNotExist(err) }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	state := t.TempDir()
	token, err := localapi.EnsureToken(state)
	if err != nil {
		t.Fatal(err)
	}
	network, addr := daemon.SocketPath(state)
	api, err := net.Listen(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	peer := daemon.SessionPeer{Name: "fixture-host", IPv4: "127.0.0.1", PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
	server := &http.Server{Handler: localapi.RequireToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/localapi/status":
			json.NewEncoder(w).Encode(&daemon.Status{Status: "up", NodeName: "fixture-viewer", Peers: []daemon.PeerStatus{{Name: peer.Name, IPv4: peer.IPv4}}})
		case "/localapi/session-peers":
			if current() {
				json.NewEncoder(w).Encode([]daemon.SessionPeer{peer})
			} else {
				json.NewEncoder(w).Encode([]daemon.SessionPeer{})
			}
		default:
			http.NotFound(w, r)
		}
	}))}
	go server.Serve(api)
	defer server.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:5909")
	if err != nil {
		t.Fatal("owned fixture session port unavailable", err)
	}
	defer ln.Close()
	ui, err := newAttendedUI()
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, "viewer.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, viewerBin, "--state", state, "session", "view", peer.Name)
	cmd.Env = append([]string{}, os.Environ()...)
	for i, v := range cmd.Env {
		if strings.HasPrefix(v, "DISPLAY=") {
			cmd.Env[i] = "DISPLAY=" + viewerDisplay
		}
	}
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	stage := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "stage"), []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	stage("view")
	host, err := listenAttendedSessionWithApproval(ln, peer.Name, 15*time.Second, func(h session.Hello, from net.Addr, conn net.Conn) (bool, string) {
		if h.NodeName != "fixture-viewer" {
			return false, "wrong fixture peer"
		}
		// No host capture/input exists until the real local prompt accepts.
		return approveAttendedConnection(ctx, ui, "Fixture native view-only client", conn, current), "local viewing decision"
	})
	if mode == "pending-close" || mode == "decline" {
		if err == nil {
			host.Close()
			t.Fatal("retired/declined view reached capture")
		}
		err = cmd.Wait()
		if mode == "pending-close" && err != nil {
			t.Fatal(err)
		}
		if mode == "decline" && err == nil {
			t.Fatal("decline did not report refusal")
		}
		stage("done")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	rawCapture, err := newCapturer(state)
	if err != nil {
		t.Fatal(err)
	}
	safety, err := newAttendedSafety()
	if err != nil {
		t.Fatal(err)
	}
	defer safety.Close()
	var injected atomic.Int32
	capture := &observedViewCapture{Capturer: rawCapture}
	_, sessionDone := serveAttendedSession(ctx, host, "Fixture native view-only client", false, ui, capture, func() (session.Injector, error) {
		injected.Add(1)
		return nil, errors.New("view-only must not create input")
	}, attendedSafetyAuthorized(safety, current))
	done := bindAttendedSafety(ctx, host, safety, sessionDone, func(err error) { t.Errorf("native safety cleanup: %v", err) })
	stage("live")
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err = <-exited:
		if mode == "local-end" && err != nil {
			t.Fatal(err)
		}
		if mode != "local-end" && err == nil {
			t.Fatal("failure path did not report viewer termination")
		}
	case <-ctx.Done():
		t.Fatal("viewer did not stop", ctx.Err())
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("host owned capture/UI not joined")
	}
	if injected.Load() != 0 {
		t.Fatal("view-only created host injector")
	}
	if mode == "local-end" && capture.deltas.Load() == 0 {
		t.Fatal("actual captured desktop delta was not sent")
	}
	stage("done")
}

type observedViewCapture struct {
	session.Capturer
	deltas atomic.Int32
}

func (c *observedViewCapture) CaptureDelta() ([]byte, []session.Tile, error) {
	source, ok := c.Capturer.(session.DeltaCapturer)
	if !ok {
		return nil, nil, errors.New("fixture requires actual delta capturer")
	}
	full, tiles, err := source.CaptureDelta()
	if err == nil && len(full) == 0 && len(tiles) > 0 {
		c.deltas.Add(1)
	}
	return full, tiles, err
}
func (c *observedViewCapture) Close() error {
	if closer, ok := c.Capturer.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
