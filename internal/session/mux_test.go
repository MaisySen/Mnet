package session

import (
	"bytes"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

func pipe(t *testing.T) (client, host *Mux) {
	t.Helper()
	c1, c2 := net.Pipe()
	ln := &oneShotListener{ch: make(chan net.Conn, 1)}
	ln.ch <- c2
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		host, err = Listen(ln, "host-node", func(h Hello) (bool, string) {
			if h.NodeName != "viewer-node" {
				t.Errorf("hello node name %q", h.NodeName)
			}
			return true, ""
		})
		if err != nil {
			t.Errorf("listen: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		client, err = Dial(c1, "viewer-node")
		if err != nil {
			t.Errorf("dial: %v", err)
		}
	}()
	wg.Wait()
	return client, host
}

type oneShotListener struct {
	ch chan net.Conn
}

func (l *oneShotListener) Accept() (net.Conn, error) { return <-l.ch, nil }
func (l *oneShotListener) Close() error              { return nil }
func (l *oneShotListener) Addr() net.Addr            { return nil }

func TestHandshakeRefused(t *testing.T) {
	c1, c2 := net.Pipe()
	ln := &oneShotListener{ch: make(chan net.Conn, 1)}
	ln.ch <- c2
	go Listen(ln, "host", func(Hello) (bool, string) { return false, "not today" })
	if _, err := Dial(c1, "viewer"); err == nil {
		t.Fatal("expected refusal")
	} else if got := err.Error(); got != "session: refused: not today" {
		t.Fatalf("error %q", got)
	}
}

func TestChatRoundTrip(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()

	got := make(chan ChatMessage, 1)
	host.OnStream(func(s Stream) {
		if s.ID != StreamChat {
			return
		}
		f, err := s.Recv(2 * time.Second)
		if err != nil {
			t.Errorf("host recv: %v", err)
			return
		}
		var m ChatMessage
		if err := json.Unmarshal(f.Payload, &m); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		got <- m
	})
	// Viewer opens the chat stream by sending an OPEN, then DATA.
	if err := client.send(StreamChat, msgOpen, PriChat, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	want := ChatMessage{From: "viewer-node", Text: "hello host", At: 123}
	if err := client.SendJSON(StreamChat, PriChat, want); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != want {
			t.Fatalf("got %+v want %+v", m, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for chat")
	}
}

func TestPriorityDrainsFirst(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()

	// Fill the host-bound pipe with bulk data, then queue interactive.
	// net.Pipe is synchronous, so drain on the host concurrently.
	received := make(chan Frame, 64)
	host.OnStream(func(s Stream) {})
	go func() {
		for {
			f, err := readFrame(host.conn)
			if err != nil {
				return
			}
			if f.Type == msgData {
				received <- f
			}
		}
	}()

	// Queue a bulk frame and an interactive frame while the writer is
	// blocked, then check the interactive one arrives first. To force
	// ordering, hold the mux write mutex around both sends.
	client.wmu.Lock()
	big := bytes.Repeat([]byte("x"), 1024)
	client.pending[PriBulk] = append(client.pending[PriBulk],
		Frame{StreamID: StreamFile, Type: msgData, Priority: PriBulk, Payload: big},
		Frame{StreamID: StreamFile, Type: msgData, Priority: PriBulk, Payload: big})
	client.pending[PriInteractive] = append(client.pending[PriInteractive],
		Frame{StreamID: StreamInput, Type: msgData, Priority: PriInteractive, Payload: []byte(`{"kind":"key","text":"a"}`)})
	client.wmu.Unlock()
	client.cond.Signal()

	first := <-received
	if first.StreamID != StreamInput {
		t.Fatalf("first frame was stream %d, want interactive input", first.StreamID)
	}
}

func TestDynamicStreamFileTransfer(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()

	done := make(chan int, 1)
	host.OnStream(func(s Stream) {
		if s.ID < StreamFile {
			return
		}
		var offer FileOffer
		if err := json.Unmarshal(s.OpenJSON, &offer); err != nil {
			t.Errorf("offer: %v", err)
			return
		}
		if offer.Name != "report.pdf" || offer.Size != 4096 {
			t.Errorf("offer %+v", offer)
		}
		n := 0
		for {
			f, err := s.Recv(time.Second)
			if err != nil {
				break
			}
			n += len(f.Payload)
		}
		done <- n
	})

	offer := FileOffer{Name: "report.pdf", Size: 4096, SHA256: "00"}
	s, err := client.Open(PriBulk, offer)
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("z"), 1024)
	total := 0
	for i := 0; i < 4; i++ {
		if err := client.SendBytes(s.ID, PriBulk, chunk); err != nil {
			t.Fatal(err)
		}
		total += len(chunk)
	}
	client.send(s.ID, msgFin, PriBulk, nil)
	select {
	case n := <-done:
		if n != total {
			t.Fatalf("received %d bytes, want %d", n, total)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Frame{StreamID: 7, Type: msgData, Priority: PriScreen, Payload: []byte("payload")}
	if err := writeFrame(&buf, in); err != nil {
		t.Fatal(err)
	}
	out, err := readFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if out.StreamID != in.StreamID || out.Type != in.Type || out.Priority != in.Priority ||
		!bytes.Equal(out.Payload, in.Payload) {
		t.Fatalf("round trip %+v want %+v", out, in)
	}
	if err := writeFrame(&buf, Frame{Payload: make([]byte, maxPayload+1)}); err == nil {
		t.Fatal("expected oversize error")
	}
}
