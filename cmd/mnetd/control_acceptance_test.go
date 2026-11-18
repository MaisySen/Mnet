package main

// Manual-only disposable attended-control acceptance counterpart. No production
// endpoint includes this test. The loopback command server is not shipped.
import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

type acceptanceScreen struct {
	tick   atomic.Uint32
	inputs atomic.Uint32
}

func (s *acceptanceScreen) Geometry() (int, int) { return 320, 480 }
func (s *acceptanceScreen) Capture() ([]byte, bool, error) {
	im := image.NewRGBA(image.Rect(0, 0, 320, 480))
	n := s.tick.Add(1)
	for y := 0; y < 480; y++ {
		for x := 0; x < 320; x++ {
			im.SetRGBA(x, y, color.RGBA{30, byte(40 + n%80), 80, 255})
		}
	}
	var b bytes.Buffer
	err := jpeg.Encode(&b, im, nil)
	return b.Bytes(), true, err
}
func (s *acceptanceScreen) Inject(e session.InputEvent) error { s.inputs.Add(1); return nil }

type acceptancePeer struct {
	mu           sync.Mutex
	peer         *proofNodeT
	mux          *session.Mux
	viewer       *session.Viewer
	vc           *session.ViewerControl
	hc           *session.HostControl
	host         *session.Host
	screen       *acceptanceScreen
	frames       atomic.Uint32
	old          session.ControlStatus
	staleRefused bool
}

func (p *acceptancePeer) currentAndroid() (daemon.SessionPeer, bool) {
	for _, v := range p.peer.d.SessionPeers() {
		if v.Name == "android-emulator" {
			return v, true
		}
	}
	return daemon.SessionPeer{}, false
}
func (p *acceptancePeer) close() {
	if p.host != nil {
		p.host.Stop()
		p.host = nil
	}
	if p.mux != nil {
		p.mux.Close()
	}
	p.mux = nil
	p.viewer = nil
	p.vc = nil
	p.hc = nil
}
func (p *acceptancePeer) command(cmd string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch cmd {
	case "status":
		return nil
	case "viewer":
		p.close()
		p.frames.Store(0)
		go p.connectViewer()

	case "request":
		if p.vc == nil {
			return net.ErrClosed
		}
		return p.vc.Request()
	case "cancel":
		if p.vc == nil {
			return net.ErrClosed
		}
		return p.vc.Cancel()
	case "tap", "rawtap":
		if p.viewer == nil {
			return net.ErrClosed
		}
		data, err := os.ReadFile(filepath.Join(os.Getenv("MNET_CONTROL_ROOT"), "target.json"))
		if err != nil {
			return err
		}
		var point struct{ X, Y, Width, Height int }
		if json.Unmarshal(data, &point) != nil || point.Width < 1 || point.Height < 1 {
			return net.ErrClosed
		}
		g := p.viewer.Geometry()
		if g == nil {
			return net.ErrClosed
		}
		event := session.InputEvent{Kind: "pointer", Action: "click", Button: "left", X: point.X * g.Width / point.Width, Y: point.Y * g.Height / point.Height}
		if cmd == "rawtap" {
			st := p.vc.Snapshot()
			event.ControlSession = st.Session
			event.ControlRequest = st.Request
			return p.mux.SendJSON(session.StreamInput, session.PriInteractive, event)
		}
		return p.viewer.SendInput(event)
	case "host", "unsupported":
		p.close()
		p.screen = &acceptanceScreen{}
		p.staleRefused = false
		go p.acceptHost(cmd == "host")
	case "allow", "decline", "keep", "pause", "revoke", "stale":
		if p.hc == nil {
			return net.ErrClosed
		}
		s := p.hc.Snapshot()
		switch cmd {
		case "allow", "keep":
			p.old = s
			return p.hc.Decide(s.Session, s.Request, true)
		case "decline":
			p.old = s
			return p.hc.Decide(s.Session, s.Request, false)
		case "pause":
			return p.hc.Pause(s.Session, s.Request)
		case "revoke":
			return p.hc.Revoke(s.Session, s.Request)
		case "stale":
			p.staleRefused = p.hc.Decide(p.old.Session, p.old.Request, true) != nil
			return nil
		}
	case "close":
		p.close()
	default:
		return net.ErrClosed
	}
	return nil
}
func (p *acceptancePeer) connectViewer() {
	target, ok := p.currentAndroid()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, err := p.peer.net.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(netip.MustParseAddr(target.IPv4), 5909))
	if err != nil {
		return
	}
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	m, err := session.DialWithControl(conn, "control-peer", func(r session.HelloReply, from net.Addr) error {
		now, ok := p.currentAndroid()
		if !ok || now.PublicKey != target.PublicKey || r.NodeName != "android-emulator" {
			return net.ErrClosed
		}
		return nil
	}, true)
	if err != nil {
		return
	}
	conn.SetDeadline(time.Time{})
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mux = m
	p.vc = session.NewViewerControl(nil)
	p.viewer = session.ConnectViewerWithOptions(m, session.ViewerOptions{Control: p.vc, OnFrame: func(f session.ScreenFrame) {
		if _, err := jpeg.Decode(bytes.NewReader(f.Image)); err == nil {
			p.frames.Add(1)
		}
	}})
}
func (p *acceptancePeer) acceptHost(capable bool) {
	ln, err := p.peer.net.ListenTCPAddrPort(netip.AddrPortFrom(p.peer.ip, 5909))
	if err != nil {
		return
	}
	defer ln.Close()
	// The listener deadline is owned by the outer bounded test process.
	m, err := session.Listen(ln, "control-peer", func(h session.Hello, from net.Addr) (bool, string) {
		target, ok := p.currentAndroid()
		if !ok || h.NodeName != "android-emulator" {
			return false, "denied"
		}
		name, _, matched := daemon.MatchSessionPeer(p.peer.d.SessionPeers(), h.NodeName, from)
		return matched && name == target.Name, "denied"
	})
	if err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mux = m
	p.hc = session.NewHostControl(nil)
	p.host = session.ServeHostWithOptions(m, p.screen, p.screen, 8, session.HostOptions{Control: p.hc, NegotiateControl: capable})
}
func (p *acceptancePeer) status() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := session.ControlStatus{State: "unavailable"}
	if p.vc != nil {
		state = p.vc.Snapshot()
	}
	if p.hc != nil {
		state = p.hc.Snapshot()
	}
	closed := true
	if p.mux != nil {
		select {
		case <-p.mux.Closed():
		default:
			closed = false
		}
	}
	inputs := uint32(0)
	if p.screen != nil {
		inputs = p.screen.inputs.Load()
	}
	return map[string]any{"control": state, "frames": p.frames.Load(), "inputs": inputs, "closed": closed, "stale_refused": p.staleRefused}
}
func TestControlAcceptancePeer(t *testing.T) {
	path := os.Getenv("MNET_CONTROL_CONFIG")
	if path == "" {
		t.Skip("manual disposable acceptance only")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("disposable public CI only")
	}
	var cfg struct{ Server, Network, Key, Root string }
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &cfg) != nil {
		t.Fatal("private config invalid")
	}
	peer, err := newProofNode(cfg.Root, "control-peer", cfg.Server, cfg.Network, "control-peer", cfg.Key, false)
	if err != nil {
		t.Fatal("fixture setup failed")
	}
	defer peer.d.Down()
	os.Setenv("MNET_CONTROL_ROOT", cfg.Root)
	p := &acceptancePeer{peer: peer}
	defer p.close()
	http.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", 405)
			return
		}
		var v struct{ Command string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128)).Decode(&v) != nil {
			http.Error(w, "input", 400)
			return
		}
		err := p.command(v.Command)
		result := p.status()
		result["ok"] = err == nil
		json.NewEncoder(w).Encode(result)
	})
	ready, _ := json.Marshal(map[string]string{"ipv4": peer.ip.String()})
	os.WriteFile(filepath.Join(cfg.Root, "ready.json"), ready, 0600)
	server := &http.Server{Addr: "127.0.0.1:8081", ReadHeaderTimeout: 3 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		t.Fatal("fixture loopback server stopped")
	}
}

func TestControlAcceptanceSyntheticCaptureAndRefusedUnreadyCommands(t *testing.T) {
	s := &acceptanceScreen{}
	frame, changed, err := s.Capture()
	if err != nil || !changed {
		t.Fatal("capture unavailable")
	}
	im, err := jpeg.Decode(bytes.NewReader(frame))
	if err != nil || im.Bounds().Dx() != 320 || im.Bounds().Dy() != 480 {
		t.Fatal("invalid synthetic target")
	}
	if err := s.Inject(session.InputEvent{}); err != nil || s.inputs.Load() != 1 {
		t.Fatal("fixture input counter")
	}
	p := &acceptancePeer{}
	for _, cmd := range []string{"request", "cancel", "tap", "rawtap", "allow", "decline", "keep", "pause", "revoke", "stale", "unknown"} {
		if p.command(cmd) == nil {
			t.Fatalf("unready fixture command accepted: %s", cmd)
		}
	}
	status := p.status()
	if status["closed"] != true || status["inputs"] != uint32(0) {
		t.Fatal("unready status")
	}
}
