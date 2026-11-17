package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSafeFileReception(t *testing.T) {
	for _, tc := range []struct {
		name      string
		offer     string
		declared  int64
		payload   []byte
		existing  bool
		accepted  bool
		published bool
	}{
		{"complete", "ok.bin", 3, []byte("abc"), false, true, true},
		{"empty", "empty.bin", 0, nil, false, true, true},
		{"traversal", "../outside.bin", 3, []byte("abc"), false, false, false},
		{"windows-traversal", "..\\outside.bin", 3, []byte("abc"), false, false, false},
		{"negative", "bad.bin", -1, nil, false, false, false},
		{"too-large", "big.bin", 11, nil, false, false, false},
		{"short", "short.bin", 4, []byte("abc"), false, true, false},
		{"overflow", "overflow.bin", 2, []byte("abc"), false, true, false},
		{"exists", "old.bin", 3, []byte("abc"), true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			dir := t.TempDir()
			target := filepath.Join(dir, tc.offer)
			if tc.existing {
				if err := os.WriteFile(target, []byte("keep me"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			complete := make(chan struct{})
			host.OnStream(func(s Stream) { defer close(complete); FileReceiver{Mux: host, Dir: dir, MaxBytes: 10}.Handle(s) })
			digest := sha256.Sum256(tc.payload)
			s, err := client.Open(PriBulk, FileOffer{Name: tc.offer, Size: tc.declared, SHA256: hex.EncodeToString(digest[:])})
			if err != nil {
				t.Fatal(err)
			}
			f, err := s.Recv(10 * time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var accept FileAccept
			if err = json.Unmarshal(f.Payload, &accept); err != nil {
				t.Fatal(err)
			}
			if accept.OK != tc.accepted {
				t.Fatalf("accept=%+v", accept)
			}
			if accept.OK {
				if err = client.SendBytes(s.ID, PriBulk, tc.payload); err != nil {
					t.Fatal(err)
				}
				if err = client.send(s.ID, msgFin, PriBulk, nil); err != nil {
					t.Fatal(err)
				}
				f, err = s.Recv(10 * time.Second)
				if err != nil {
					t.Fatal(err)
				}
				var ack FileAck
				if err = json.Unmarshal(f.Payload, &ack); err != nil {
					t.Fatal(err)
				}
				if ack.OK != tc.published {
					t.Fatalf("ack=%+v", ack)
				}
			}
			select {
			case <-complete:
			case <-time.After(10 * time.Second):
				t.Fatal("receiver did not finish")
			}
			got, err := os.ReadFile(target)
			switch {
			case tc.existing:
				if err != nil || string(got) != "keep me" {
					t.Fatal("existing destination overwritten")
				}
			case tc.published:
				if err != nil || !bytes.Equal(got, tc.payload) {
					t.Fatalf("published bytes=%q err=%v", got, err)
				}
				info, _ := os.Stat(target)
				if info.Mode().Perm() != 0600 {
					t.Fatalf("file mode=%v", info.Mode())
				}
			default:
				if !os.IsNotExist(err) {
					t.Fatalf("partial/rejected target exists: %v", err)
				}
			}
			leftovers, err := filepath.Glob(filepath.Join(dir, ".mnet-transfer-*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary files left: %v %v", leftovers, err)
			}
		})
	}
}

func TestRepeatedFileTransfersReleaseStreams(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	dir := t.TempDir()
	sourceDir := t.TempDir()
	host.OnStream(func(s Stream) { FileReceiver{Mux: host, Dir: dir}.Handle(s) })
	for i := 0; i < maxActiveStreams+10; i++ {
		source := filepath.Join(sourceDir, fmt.Sprintf("file-%d", i))
		if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := SendFile(client, source); err != nil {
			t.Fatalf("transfer %d: %v", i, err)
		}
	}
	client.streamMu.Lock()
	count := len(client.streams)
	client.streamMu.Unlock()
	if count != 0 {
		t.Fatalf("completed outgoing streams leaked: %d", count)
	}
	client.wmu.Lock()
	opened := len(client.opened)
	client.wmu.Unlock()
	if opened > 1 {
		t.Fatalf("completed outgoing markers leaked: %d", opened)
	}
}

func TestResetTransferDoesNotPublish(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	dir := t.TempDir()
	finished := make(chan struct{})
	host.OnStream(func(s Stream) { defer close(finished); FileReceiver{Mux: host, Dir: dir}.Handle(s) })
	s, err := client.Open(PriBulk, FileOffer{Name: "reset.bin", Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Recv(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err = client.SendBytes(s.ID, PriBulk, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err = client.send(s.ID, msgRst, PriBulk, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("reset not observed")
	}
	if _, err = os.Stat(filepath.Join(dir, "reset.bin")); !os.IsNotExist(err) {
		t.Fatal("reset published a file")
	}
}

func TestIdleFileReceiverCleansPartialAndReleasesStream(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	dir := t.TempDir()
	finished := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(finished)
		FileReceiver{Mux: host, Dir: dir, IdleTimeout: 40 * time.Millisecond}.Handle(s)
	})
	stream, err := client.Open(PriBulk, FileOffer{Name: "unfinished.bin", Size: 6})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(time.Second); err != nil {
		t.Fatal(err)
	}
	if err := client.SendBytes(stream.ID, PriBulk, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("idle receiver retained partial transfer")
	}
	if _, err := stream.Recv(time.Second); !errors.Is(err, ErrStreamReset) {
		t.Fatalf("sender was not told idle transfer reset: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial files remain: %v %v", entries, err)
	}
	host.streamMu.Lock()
	_, retained := host.streams[stream.ID]
	host.streamMu.Unlock()
	if retained {
		t.Fatal("idle receiver retained stream")
	}
	select {
	case <-host.Closed():
		t.Fatal("idle file closed unrelated session")
	default:
	}
}

func TestEmptyFileFramesDoNotExtendIdleBudget(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	dir := t.TempDir()
	finished := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(finished)
		FileReceiver{Mux: host, Dir: dir, IdleTimeout: 50 * time.Millisecond}.Handle(s)
	})
	stream, err := client.Open(PriBulk, FileOffer{Name: "empty-keepalive.bin", Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Recv(time.Second); err != nil {
		t.Fatal(err)
	}
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	limit := time.NewTimer(300 * time.Millisecond)
	defer limit.Stop()
	for {
		select {
		case <-finished:
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatal("partial file retained")
			}
			return
		case <-tick.C:
			if err = client.SendBytes(stream.ID, PriBulk, nil); err != nil {
				t.Fatal(err)
			}
		case <-limit.C:
			t.Fatal("empty frames kept file reception alive")
		}
	}
}

func TestFileProgressRestartsIdleBudget(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	dir := t.TempDir()
	finished := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(finished)
		FileReceiver{Mux: host, Dir: dir, IdleTimeout: 200 * time.Millisecond}.Handle(s)
	})
	stream, err := client.Open(PriBulk, FileOffer{Name: "progress.bin", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Recv(time.Second); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(60 * time.Millisecond)
		if err = client.SendBytes(stream.ID, PriBulk, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err = client.send(stream.ID, msgFin, PriBulk, nil); err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var ack FileAck
	if json.Unmarshal(frame.Payload, &ack) != nil || !ack.OK {
		t.Fatal("progressing transfer refused")
	}
	<-finished
	data, err := os.ReadFile(filepath.Join(dir, "progress.bin"))
	if err != nil || string(data) != "xxxxx" {
		t.Fatal("progress file missing")
	}
}

// Saturate a started mux without a writer goroutine so the queue cannot drain.
// This is a deterministic queue-boundary fixture, not a real-network timing test.
func TestIdleFileReceiverCleansWithSaturatedOutboundQueue(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	m := newMux(a, StreamFile)
	defer m.Close()
	m.started = true
	id := uint32(5)
	offer, _ := json.Marshal(FileOffer{Name: "blocked.bin", Size: 6})
	in := make(chan Frame, 1)
	in <- Frame{Type: msgData, Payload: []byte("abc")}
	m.streams[id] = &streamState{ch: in, handled: true}
	dir := t.TempDir()
	done := make(chan struct{})
	go func() {
		defer close(done)
		FileReceiver{Mux: m, Dir: dir, IdleTimeout: 100 * time.Millisecond}.Handle(Stream{Mux: m, ID: id, OpenJSON: offer, in: in})
	}()
	deadline := time.Now().Add(time.Second)
	for {
		m.wmu.Lock()
		accepted := m.queuedBytes > 0
		if accepted {
			m.pending = [4][]Frame{}
			m.pending[PriBulk] = []Frame{{StreamID: 99, Type: msgData, Priority: PriBulk, Payload: make([]byte, maxQueuedBytes-headerLen)}}
			m.queuedBytes = maxQueuedBytes
		}
		m.wmu.Unlock()
		if accepted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("receiver did not queue acceptance")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full outbound queue blocked idle cleanup")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial files remain: %v %v", entries, err)
	}
	m.streamMu.Lock()
	_, retained := m.streams[id]
	m.streamMu.Unlock()
	if retained {
		t.Fatal("idle stream retained")
	}
	m.wmu.Lock()
	queued := m.queuedBytes
	frames := len(m.pending[PriBulk])
	m.wmu.Unlock()
	if queued != maxQueuedBytes || frames != 1 {
		t.Fatalf("reset exceeded saturated queue: bytes=%d frames=%d", queued, frames)
	}
	select {
	case <-m.Closed():
		t.Fatal("idle cleanup closed unrelated mux")
	default:
	}
}

func TestFileRefusalDoesNotDiscloseReceiverStoragePath(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	root := t.TempDir()
	privatePath := filepath.Join(root, "private-account-directory")
	if err := os.WriteFile(privatePath, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, Dir: filepath.Join(privatePath, "private-subdirectory")}.Handle(s)
	})
	stream, err := client.Open(PriBulk, FileOffer{Name: "public-offer.bin", Size: 0})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var refusal FileAccept
	if err = json.Unmarshal(frame.Payload, &refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.OK || refusal.Reason != "receiver storage unavailable" {
		t.Fatalf("unsafe refusal: %+v", refusal)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("receiver did not stop")
	}
}
