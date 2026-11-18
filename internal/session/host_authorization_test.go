package session

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type authorizationCapturer struct{ captures atomic.Int32 }

func (c *authorizationCapturer) Geometry() (int, int) { return 16, 16 }
func (c *authorizationCapturer) Capture() ([]byte, bool, error) {
	c.captures.Add(1)
	return nil, false, nil
}
func TestHostKnownAuthorizationLossClosesActiveSession(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	var authorized atomic.Bool
	authorized.Store(true)
	cap := &authorizationCapturer{}
	h := ServeHostWithOptions(host, cap, nil, 8, HostOptions{Authorized: authorized.Load})
	defer h.Stop()
	until := time.Now().Add(time.Second)
	for cap.captures.Load() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if cap.captures.Load() == 0 {
		t.Fatal("capture did not start")
	}
	authorized.Store(false)
	select {
	case <-client.Closed():
	case <-time.After(time.Second):
		t.Fatal("known denied desktop session remained alive")
	}
}

type blockedAuthorizationCapturer struct {
	entered, release chan struct{}
	closes           atomic.Int32
}

func (c *blockedAuthorizationCapturer) Geometry() (int, int) { return 16, 16 }
func (c *blockedAuthorizationCapturer) Capture() ([]byte, bool, error) {
	close(c.entered)
	<-c.release
	return []byte("private frame"), true, nil
}
func (c *blockedAuthorizationCapturer) Close() error { c.closes.Add(1); return nil }

func TestHostAuthorizationClosesTransportWhileCaptureBlocked(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	var authorized atomic.Bool
	authorized.Store(true)
	cap := &blockedAuthorizationCapturer{entered: make(chan struct{}), release: make(chan struct{})}
	h := ServeHostWithOptions(host, cap, nil, 8, HostOptions{Authorized: authorized.Load})
	defer h.Stop()
	select {
	case <-cap.entered:
	case <-time.After(time.Second):
		close(cap.release)
		t.Fatal("capture did not enter")
	}
	authorized.Store(false)
	select {
	case <-client.Closed():
	case <-time.After(time.Second):
		close(cap.release)
		t.Fatal("blocked capture retained transport")
	}
	close(cap.release)
	h.Stop()
	if cap.closes.Load() != 1 {
		t.Fatal("capture closer not preserved", cap.closes.Load())
	}
	authorized.Store(true)
	if h.allowed() {
		t.Fatal("reapproved map revived old host")
	}
}

type authorizationInjector struct{ calls atomic.Int32 }

func (i *authorizationInjector) Inject(InputEvent) error { i.calls.Add(1); return nil }
func TestHostAuthorizationBlocksQueuedInputAndChat(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	var authorized atomic.Bool
	authorized.Store(true)
	inj := &authorizationInjector{}
	var chats atomic.Int32
	h := ServeHostWithOptions(host, &authorizationCapturer{}, inj, 8, HostOptions{Authorized: authorized.Load, OnChat: func(ChatMessage) { chats.Add(1) }})
	defer h.Stop()
	v := ConnectViewer(client)
	if err := v.SendInput(InputEvent{Kind: "pointer", Action: "click"}); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	for inj.calls.Load() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if inj.calls.Load() != 1 {
		t.Fatal("authorized input lost")
	}
	authorized.Store(false)
	_ = v.SendInput(InputEvent{Kind: "pointer", Action: "click"})
	_ = v.SendChat("viewer", "private")
	select {
	case <-client.Closed():
	case <-time.After(time.Second):
		t.Fatal("denial did not close")
	}
	h.Stop()
	if inj.calls.Load() != 1 || chats.Load() != 0 {
		t.Fatal("known denied dispatch", inj.calls.Load(), chats.Load())
	}
}

func TestFileAuthorizationLossRemovesPrivatePartial(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	var authorized atomic.Bool
	authorized.Store(true)
	dir := t.TempDir()
	done := make(chan struct{})
	var notified atomic.Int32
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, Dir: dir, Authorized: authorized.Load, OnDone: func(string, int64) { notified.Add(1) }}.Handle(s)
	})
	s, err := client.Open(PriBulk, FileOffer{Name: "private.bin", Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var accepted FileAccept
	if err := json.Unmarshal(f.Payload, &accepted); err != nil || !accepted.OK {
		t.Fatal("not accepted", err)
	}
	authorized.Store(false)
	if err := client.SendBytes(s.ID, PriBulk, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	_ = client.send(s.ID, msgFin, PriBulk, nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("denied file retained receiver")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("denied transfer retained data", entries, err)
	}
	if notified.Load() != 0 {
		t.Fatal("denied file completion notified")
	}
}

type revokingFullCapture struct {
	authorized *atomic.Bool
	jpeg       []byte
}

func (c *revokingFullCapture) Geometry() (int, int) { return 16, 16 }
func (c *revokingFullCapture) Capture() ([]byte, bool, error) {
	c.authorized.Store(false)
	return c.jpeg, true, nil
}

type revokingDeltaCapture struct {
	revokingFullCapture
	fallback      bool
	fallbackCalls atomic.Int32
}

func (c *revokingDeltaCapture) CaptureDelta() ([]byte, []Tile, error) {
	c.authorized.Store(false)
	if c.fallback {
		return nil, []Tile{{Rect: Rect{W: 16, H: 16}, Image: c.jpeg}}, nil
	}
	return c.jpeg, nil, nil
}
func (c *revokingDeltaCapture) Capture() ([]byte, bool, error) {
	c.fallbackCalls.Add(1)
	return c.jpeg, true, nil
}
func TestHostDropsCaptureAndRecordingAfterKnownDenial(t *testing.T) {
	for _, mode := range []string{"full", "delta", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			var authorized atomic.Bool
			authorized.Store(true)
			full := revokingFullCapture{authorized: &authorized, jpeg: jpegBytes(t, solid(16, 16, color.RGBA{R: 50, A: 255}))}
			var cap Capturer = &full
			delta := &revokingDeltaCapture{revokingFullCapture: full, fallback: mode == "fallback"}
			if mode != "full" {
				cap = delta
			}
			rec, err := NewRecorder(filepath.Join(t.TempDir(), "screen.mjpeg"))
			if err != nil {
				t.Fatal(err)
			}
			defer rec.Close()
			h := ServeHostWithOptions(host, cap, nil, 8, HostOptions{Authorized: authorized.Load, Recorder: rec})
			defer h.Stop()
			select {
			case <-client.Closed():
			case <-time.After(time.Second):
				t.Fatal("denial during capture retained transport")
			}
			h.Stop()
			if rec.Frames() != 0 {
				t.Fatal("revoked capture reached recorder")
			}
			if delta.fallbackCalls.Load() != 0 {
				t.Fatal("authorization loss dispatched fallback capture")
			}
		})
	}
}
