package session

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeCapturer struct{ img []byte }

func (f fakeCapturer) Capture() ([]byte, bool, error) { return f.img, true, nil }
func (f fakeCapturer) Geometry() (int, int)           { return 320, 240 }

type fakeInjector struct {
	mu  sync.Mutex
	evs []InputEvent
}

func (f *fakeInjector) Inject(ev InputEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evs = append(f.evs, ev)
	return nil
}

func TestFullSession(t *testing.T) {
	c1, c2 := net.Pipe()
	ln := &oneShotListener{ch: make(chan net.Conn, 1)}
	ln.ch <- c2

	inj := &fakeInjector{}
	fileDir := t.TempDir()
	chatAtHost := make(chan ChatMessage, 1)
	var host *Host
	var hostErr error
	go func() {
		var m *Mux
		m, hostErr = Listen(ln, "desk-1", func(Hello, net.Addr) (bool, string) { return true, "" })
		if hostErr != nil {
			return
		}
		host = ServeHost(m, fakeCapturer{img: []byte("jpeg-bytes")}, inj, 20)
		host.SetOnChat(func(cm ChatMessage) { chatAtHost <- cm })
		host.SetFileDir(fileDir)
	}()

	m, err := Dial(c1, "laptop-1")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	v := ConnectViewer(m)

	frames := make(chan ScreenFrame, 1)
	chatAtViewer := make(chan ChatMessage, 1)
	v.SetOnFrame(func(fr ScreenFrame) { frames <- fr })
	v.SetOnChat(func(cm ChatMessage) { chatAtViewer <- cm })

	// Screen: host streams frames, viewer decodes them.
	select {
	case fr := <-frames:
		if string(fr.Image) != "jpeg-bytes" || fr.Seq == 0 {
			t.Fatalf("bad frame %+v", fr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no screen frame")
	}
	g := v.Geometry()
	if g == nil || g.Width != 320 || g.Height != 240 || g.Codec != "jpeg" {
		t.Fatalf("geometry %+v", g)
	}

	// Input: viewer sends events, host injector applies them.
	ev := InputEvent{Kind: "pointer", X: 10, Y: 20, Action: "click", Button: "left"}
	if err := v.SendInput(ev); err != nil {
		t.Fatal(err)
	}
	if err := v.SendInput(InputEvent{Kind: "key", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		inj.mu.Lock()
		n := len(inj.evs)
		inj.mu.Unlock()
		if n == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	inj.mu.Lock()
	if len(inj.evs) != 2 || inj.evs[0] != ev || inj.evs[1].Text != "hi" {
		t.Fatalf("injected %+v", inj.evs)
	}
	inj.mu.Unlock()

	// Chat both ways.
	if err := v.SendChat("laptop-1", "can you see this"); err != nil {
		t.Fatal(err)
	}
	select {
	case cm := <-chatAtHost:
		if cm.Text != "can you see this" {
			t.Fatalf("host chat %+v", cm)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no chat at host")
	}
	if err := host.SendChat("desk-1", "yes"); err != nil {
		t.Fatal(err)
	}
	select {
	case cm := <-chatAtViewer:
		if cm.Text != "yes" {
			t.Fatalf("viewer chat %+v", cm)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no chat at viewer")
	}

	// File: viewer pushes a file to the host, digest verified.
	src := filepath.Join(t.TempDir(), "notes.txt")
	content := bytes.Repeat([]byte("mnet session file\n"), 4096)
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 1)
	host.SetOnFile(func(name string, size int64) { received <- name })
	if err := SendFile(m, src); err != nil {
		t.Fatal(err)
	}
	select {
	case name := <-received:
		got, err := os.ReadFile(filepath.Join(fileDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, content) {
			t.Fatal("file content mismatch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("file not received")
	}

	host.Stop()
}
