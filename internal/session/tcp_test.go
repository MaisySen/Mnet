package session

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The mux over a real TCP socket: net.Pipe is synchronous, TCP adds
// kernel buffering and segmentation. Frame reassembly and priority
// draining must survive both.
func TestSessionOverTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	inj := &fakeInjector{}
	fileDir := t.TempDir()
	hostReady := make(chan *Host, 1)
	go func() {
		m, err := Listen(ln, "tcp-host", func(Hello) (bool, string) { return true, "" })
		if err != nil {
			t.Errorf("listen: %v", err)
			return
		}
		h := ServeHost(m, fakeCapturer{img: bytes.Repeat([]byte{0xff, 0xd8, 0xff}, 40000)}, inj, 10)
		h.SetFileDir(fileDir)
		hostReady <- h
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m, err := Dial(conn, "tcp-viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	v := ConnectViewer(m)

	frames := make(chan ScreenFrame, 4)
	v.SetOnFrame(func(fr ScreenFrame) { frames <- fr })
	select {
	case fr := <-frames:
		if len(fr.Image) != 120000 {
			t.Fatalf("frame size %d", len(fr.Image))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no frame over TCP")
	}

	if err := v.SendInput(InputEvent{Kind: "key", Text: "tcp"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		inj.mu.Lock()
		n := len(inj.evs)
		inj.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	inj.mu.Lock()
	if len(inj.evs) != 1 || inj.evs[0].Text != "tcp" {
		t.Fatalf("injected %+v", inj.evs)
	}
	inj.mu.Unlock()

	src := filepath.Join(t.TempDir(), "big.bin")
	content := make([]byte, 3*1024*1024) // 3 MiB: exercises segmentation
	for i := range content {
		content[i] = byte(i * 31)
	}
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	host := <-hostReady
	done := make(chan string, 1)
	host.SetOnFile(func(name string, size int64) { done <- name })
	if err := SendFile(m, src); err != nil {
		t.Fatal(err)
	}
	select {
	case name := <-done:
		got, err := os.ReadFile(filepath.Join(fileDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, content) {
			t.Fatal("3 MiB file content mismatch over TCP")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("file not received")
	}
	host.Stop()
}
