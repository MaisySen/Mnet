package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type controlledFileReader struct {
	reads int
	read  func([]byte) (int, error)
}

func (r *controlledFileReader) Read(p []byte) (int, error) { r.reads++; return r.read(p) }
func TestOwnedFileCopyCancellationAndExactSize(t *testing.T) {
	closed := make(chan struct{})
	src := bytes.NewBufferString("original-appended")
	var dst bytes.Buffer
	if err := copyOwnedFileBytes(closed, &dst, src, 8); err != nil {
		t.Fatal(err)
	}
	if dst.String() != "original" || src.String() != "-appended" {
		t.Fatal("read beyond snapshot", dst.String(), src.String())
	}
	close(closed)
	reader := &controlledFileReader{read: func([]byte) (int, error) { t.Fatal("read after close"); return 0, nil }}
	if err := copyOwnedFileBytes(closed, &dst, reader, 1); !errors.Is(err, net.ErrClosed) {
		t.Fatal("canceled copy accepted", err)
	}
	if reader.reads != 0 {
		t.Fatal("closed copy entered file read")
	}
	if err := copyOwnedFileBytes(make(chan struct{}), io.Discard, bytes.NewBufferString("x"), 2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("shrink not detected", err)
	}
	if err := copyOwnedFileBytes(make(chan struct{}), io.Discard, &controlledFileReader{read: func([]byte) (int, error) { return 0, nil }}, 1); !errors.Is(err, io.ErrNoProgress) {
		t.Fatal("zero reader looped", err)
	}
}
func TestOwnedFileCopyCloseDuringReadRefusesLateBytes(t *testing.T) {
	entered, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	src := &controlledFileReader{read: func(p []byte) (int, error) { close(entered); <-release; return copy(p, []byte{1}), nil }}
	var dst bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- copyOwnedFileBytes(closed, &dst, src, 1) }()
	<-entered
	close(closed)
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatal("late bytes accepted", err)
		}
	case <-time.After(time.Second):
		t.Fatal("copy did not return")
	}
	if dst.Len() != 0 {
		t.Fatal("late data forwarded after observed close")
	}
}
func TestSendFileSourceMutationNeverSendsAppendedBytes(t *testing.T) {
	for _, kind := range []string{"grow", "shrink", "same"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.bin")
			original := []byte("original")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			a, b := net.Pipe()
			client := newMux(a, StreamFile)
			server := newMux(b, StreamFile+1)
			defer client.Close()
			defer server.Close()
			client.start()
			server.start()
			observed := make(chan []byte, 1)
			offered := make(chan FileOffer, 1)
			server.OnStream(func(stream Stream) {
				defer stream.release()
				var offer FileOffer
				if json.Unmarshal(stream.OpenJSON, &offer) != nil {
					return
				}
				offered <- offer
				changed := []byte("original-sensitive-appended")
				if kind == "shrink" {
					changed = []byte("x")
				}
				if kind == "same" {
					changed = []byte("mutated!")
				}
				if err := os.WriteFile(path, changed, 0600); err != nil {
					return
				}
				if err := server.SendJSON(stream.ID, PriBulk, FileAccept{OK: true}); err != nil {
					return
				}
				var data []byte
				for {
					frame, err := stream.Recv(time.Second)
					if err != nil {
						if kind == "same" {
							server.SendJSON(stream.ID, PriBulk, FileAck{OK: false})
						}
						observed <- data
						return
					}
					if frame.Type == msgRst {
						observed <- data
						return
					}
					data = append(data, frame.Payload...)
				}
			})
			if err := SendFile(client, path); err == nil {
				t.Fatal("mutated file reported sent")
			}
			offer := <-offered
			sum := sha256.Sum256(original)
			if offer.Size != int64(len(original)) || offer.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatal("offer changed with source")
			}
			select {
			case data := <-observed:
				if len(data) > len(original) {
					t.Fatal("appended data escaped offered snapshot", string(data))
				}
			case <-time.After(2 * time.Second):
				t.Fatal("failure did not reset stream")
			}
		})
	}
}

func TestSendFileReceiverResetStopsOnlyTransfer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	reset := make(chan struct{})
	chat := make(chan string, 1)
	destination := t.TempDir()
	host.OnStream(func(s Stream) {
		defer s.release()
		if s.ID == StreamChat {
			frame, err := s.Recv(time.Second)
			if err == nil {
				chat <- string(frame.Payload)
			}
			return
		}
		var offer FileOffer
		if json.Unmarshal(s.OpenJSON, &offer) == nil && offer.Name == "after.bin" {
			FileReceiver{Mux: host, Dir: destination}.Handle(s)
			return
		}
		if err := host.SendJSON(s.ID, PriBulk, FileAccept{OK: true}); err != nil {
			return
		}
		if _, err := s.Recv(2 * time.Second); err != nil {
			return
		}
		host.tryReset(s.ID)
		close(reset)
		// Leave the session alive, but do not consume any further file DATA.
	})
	sent := make(chan error, 1)
	go func() { sent <- SendFile(client, path) }()
	select {
	case <-reset:
	case <-time.After(3 * time.Second):
		t.Fatal("receiver did not reset")
	}
	select {
	case err := <-sent:
		if err == nil {
			t.Fatal("reset transfer reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("sender kept transfer alive after receiver reset")
	}
	select {
	case <-client.Closed():
		t.Fatal("file cancellation ended shared session")
	default:
	}
	client.wmu.Lock()
	for _, queue := range client.pending {
		for _, frame := range queue {
			if frame.StreamID >= StreamFile && frame.Type == msgData {
				client.wmu.Unlock()
				t.Fatal("canceled file DATA remains queued")
			}
		}
	}
	client.wmu.Unlock()
	if err := client.SendJSON(StreamChat, PriChat, ChatMessage{Text: "still viewing"}); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-chat:
		if !strings.Contains(text, "still viewing") {
			t.Fatal("unrelated chat changed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("file reset prevented unrelated traffic")
	}

	after := filepath.Join(t.TempDir(), "after.bin")
	if err := os.WriteFile(after, []byte("separate transfer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SendFile(client, after); err != nil {
		t.Fatal("reset poisoned later file", err)
	}
	b, err := os.ReadFile(filepath.Join(destination, "after.bin"))
	if err != nil || string(b) != "separate transfer" {
		t.Fatal("later file changed", err)
	}

}

func TestFileOutboundRetirementPreservesUnrelatedQueues(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	m := newMux(a, StreamFile)
	id := uint32(StreamFile)
	for pri := range m.pending {
		m.pending[pri] = []Frame{{StreamID: id, Type: msgData, Payload: []byte("private-file")}, {StreamID: StreamChat, Type: msgData, Payload: []byte("keep")}, {StreamID: id + 2, Type: msgData, Payload: []byte("other-file")}}
		for _, f := range m.pending[pri] {
			m.queuedBytes += headerLen + len(f.Payload)
		}
	}
	m.retireFileOutbound(id)
	total := 0
	for _, queue := range m.pending {
		if len(queue) != 2 || queue[0].StreamID != StreamChat || queue[1].StreamID != id+2 {
			t.Fatal("retired unrelated output")
		}
		for _, f := range queue {
			total += headerLen + len(f.Payload)
		}
	}
	if total != m.queuedBytes {
		t.Fatal("outbound accounting drift", total, m.queuedBytes)
	}
	m.retireFileOutbound(id)
	if total != m.queuedBytes {
		t.Fatal("retirement not idempotent")
	}
}

func TestFileSendCancellationWakesFullOutboundCapacity(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	m := newMux(a, StreamFile)
	m.started = true
	m.queuedBytes = maxQueuedBytes
	waiting := make(chan struct{})
	m.cond = sync.NewCond(fileWaitSignaler{&m.wmu, waiting})
	canceled := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- m.sendWithCancel(StreamFile, msgData, PriBulk, []byte("no"), true, canceled) }()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("send never waited for capacity")
	}
	// Holding wmu makes close+broadcast atomic with the send's wait/recheck.
	m.wmu.Lock()
	close(canceled)
	m.cond.Broadcast()
	m.wmu.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, ErrStreamReset) {
			t.Fatal("wrong cancel result", err)
		}
	case <-time.After(time.Second):
		t.Fatal("outbound wait ignored file cancellation")
	}
	if m.queuedBytes != maxQueuedBytes || len(m.pending[PriBulk]) != 0 {
		t.Fatal("canceled file enqueued")
	}
}

type fileWaitSignaler struct {
	mu      *sync.Mutex
	waiting chan struct{}
}

func (l fileWaitSignaler) Lock()   { l.mu.Lock() }
func (l fileWaitSignaler) Unlock() { close(l.waiting); l.mu.Unlock() }
