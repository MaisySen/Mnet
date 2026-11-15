package session

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A peer may send immediately after the handshake, before host platform
// capture setup finishes. Those streams must see the configured directory
// and callbacks when the host begins dispatching them.
func TestQueuedFileSeesStartupOptions(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	source := filepath.Join(t.TempDir(), "early.bin")
	payload := bytes.Repeat([]byte("early-payload"), 100)
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sent := make(chan error, 1)
	go func() { sent <- SendFile(client, source) }()
	deadline := time.Now().Add(time.Second)
	for {
		host.streamMu.Lock()
		queued := len(host.streams) > 0
		host.streamMu.Unlock()
		if queued {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stream not queued")
		}
		time.Sleep(time.Millisecond)
	}
	dir := t.TempDir()
	completed := make(chan string, 1)
	h := ServeHostWithOptions(host, fakeCapturer{img: []byte("image")}, nil, 1, HostOptions{FileDir: dir, OnFile: func(name string, n int64) { completed <- name }})
	defer h.Stop()
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("early transfer stalled")
	}
	select {
	case name := <-completed:
		if name != "early.bin" {
			t.Fatal(name)
		}
	case <-time.After(time.Second):
		t.Fatal("early completion callback lost")
	}
	got, err := os.ReadFile(filepath.Join(dir, "early.bin"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("configured directory: %v", err)
	}
}

func TestQueuedScreenSeesViewerOptions(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	if err := host.SendJSON(StreamScreen, PriScreen, ScreenFrame{Seq: 1, Full: true, Image: []byte("first-frame")}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		client.streamMu.Lock()
		queued := len(client.streams) > 0
		client.streamMu.Unlock()
		if queued {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("screen not queued")
		}
		time.Sleep(time.Millisecond)
	}
	received := make(chan ScreenFrame, 1)
	ConnectViewerWithOptions(client, ViewerOptions{OnFrame: func(fr ScreenFrame) { received <- fr }})
	select {
	case fr := <-received:
		if fr.Seq != 1 || string(fr.Image) != "first-frame" {
			t.Fatalf("frame=%+v", fr)
		}
	case <-time.After(time.Second):
		t.Fatal("first frame lost before viewer callback registration")
	}
}
