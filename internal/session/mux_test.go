package session

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		host, err = Listen(ln, "host-node", func(h Hello, _ net.Addr) (bool, string) {
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
	go Listen(ln, "host", func(Hello, net.Addr) (bool, string) { return false, "not today" })
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
	// Viewer sends a chat line; the stream OPEN is implicit.
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
	// popLocked must drain the lowest priority class first, FIFO
	// within a class.
	m := newMux(nil, StreamFile)
	big := bytes.Repeat([]byte("x"), 1024)
	m.pending[PriBulk] = append(m.pending[PriBulk],
		Frame{StreamID: StreamFile, Type: msgData, Priority: PriBulk, Payload: big},
		Frame{StreamID: StreamFile, Type: msgData, Priority: PriBulk, Payload: big})
	m.pending[PriInteractive] = append(m.pending[PriInteractive],
		Frame{StreamID: StreamInput, Type: msgData, Priority: PriInteractive, Payload: []byte("a")})
	m.pending[PriScreen] = append(m.pending[PriScreen],
		Frame{StreamID: StreamScreen, Type: msgData, Priority: PriScreen, Payload: []byte("s")})

	want := []uint32{StreamInput, StreamScreen, StreamFile, StreamFile}
	for i, w := range want {
		f, ok := m.popLocked()
		if !ok || f.StreamID != w {
			t.Fatalf("pop %d = stream %d, want %d", i, f.StreamID, w)
		}
	}
	if _, ok := m.popLocked(); ok {
		t.Fatal("queue should be empty")
	}
}

func TestLateHandlerGetsPendingStreams(t *testing.T) {
	// A stream opened by the peer before OnStream is registered must
	// be dispatched when the handler arrives, not lost.
	c1, c2 := net.Pipe()
	ln := &oneShotListener{ch: make(chan net.Conn, 1)}
	ln.ch <- c2
	hostDone := make(chan *Mux, 1)
	go func() {
		h, err := Listen(ln, "host", func(Hello, net.Addr) (bool, string) { return true, "" })
		if err == nil {
			h.SendJSON(StreamChat, PriChat, ChatMessage{From: "host", Text: "early"})
			hostDone <- h
		} else {
			hostDone <- nil
		}
	}()
	client, err := Dial(c1, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	time.Sleep(100 * time.Millisecond) // chat stream arrived with no handler
	got := make(chan ChatMessage, 1)
	client.OnStream(func(s Stream) {
		if s.ID != StreamChat {
			return
		}
		f, err := s.Recv(2 * time.Second)
		if err != nil {
			t.Errorf("recv: %v", err)
			return
		}
		var m ChatMessage
		json.Unmarshal(f.Payload, &m)
		got <- m
	})
	select {
	case m := <-got:
		if m.Text != "early" {
			t.Fatalf("got %q", m.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending stream never dispatched")
	}
	if h := <-hostDone; h != nil {
		h.Close()
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

func TestKeepaliveDetectsDeadPeer(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	m := newMux(a, StreamFile+1)
	m.start()
	m.Keepalive(50*time.Millisecond, 150*time.Millisecond)
	select {
	case <-m.Closed():
	case <-time.After(3 * time.Second):
		t.Fatal("dead peer never detected")
	}
}

func TestKeepaliveKeepsLivePeer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		Listen(ln, "host", func(Hello, net.Addr) (bool, string) { return true, "" })
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m, err := Dial(conn, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	m.Keepalive(50*time.Millisecond, 5*time.Second)
	select {
	case <-m.Closed():
		t.Fatal("live peer was closed")
	case <-time.After(400 * time.Millisecond):
	}
	m.Close()
}

func TestKeepaliveMeasuresRTT(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hostReady := make(chan *Mux, 1)
	go func() {
		m, err := Listen(ln, "host", func(Hello, net.Addr) (bool, string) { return true, "" })
		if err == nil {
			hostReady <- m
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m, err := Dial(conn, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Keepalive(20*time.Millisecond, 5*time.Second)
	deadline := time.Now().Add(2 * time.Second)
	for m.RTT() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.RTT() == 0 {
		t.Fatal("no RTT measured")
	}
	if m.RTT() > time.Second {
		t.Fatalf("loopback RTT absurd: %s", m.RTT())
	}
	h := <-hostReady
	defer h.Close()
}

func TestQualityForRTT(t *testing.T) {
	if qualityForRTT(0) != 70 {
		t.Fatal("no measurement keeps the default")
	}
	if qualityForRTT(50*time.Millisecond) != 70 {
		t.Fatal("good path keeps full quality")
	}
	if qualityForRTT(200*time.Millisecond) != 55 {
		t.Fatal("medium path steps down")
	}
	if qualityForRTT(time.Second) != 40 {
		t.Fatal("bad path drops to the floor band")
	}
}

func TestReleasedFullStreamDoesNotBlockOtherInboundStreams(t *testing.T) {
	for _, typ := range []byte{msgData} {
		t.Run(fmt.Sprintf("type%d", typ), func(t *testing.T) {
			a, b := net.Pipe()
			defer b.Close()
			m := newMux(a, StreamFile)
			defer m.Close()
			blocked := make(chan Frame, 16)
			for i := 0; i < cap(blocked); i++ {
				blocked <- Frame{Type: msgData}
			}
			other := make(chan Frame, 16)
			m.streams[5] = &streamState{ch: blocked, done: make(chan struct{}), handled: true}
			m.streams[7] = &streamState{ch: other, done: make(chan struct{}), handled: true}
			go m.readLoop()
			// A pipe write completes only after readFrame has consumed the frame.
			if err := writeFrame(b, Frame{StreamID: 5, Type: typ, Priority: PriBulk, Payload: []byte("blocked")}); err != nil {
				t.Fatal(err)
			}
			if err := b.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			next := make(chan error, 1)
			go func() {
				next <- writeFrame(b, Frame{StreamID: 7, Type: msgData, Priority: PriBulk, Payload: []byte("other")})
			}()
			// The second pipe write cannot finish while the read loop is delivering
			// the first frame into the full queue. Wait for that blocked state.
			select {
			case err := <-next:
				t.Fatalf("expected full stream to block before release: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			m.releaseStream(5)
			m.releaseStream(5) // repeated release must be safe
			select {
			case err := <-next:
				if err != nil {
					t.Fatalf("released stream still blocks read loop: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("released stream did not unblock read loop")
			}
			select {
			case f := <-other:
				if string(f.Payload) != "other" {
					t.Fatalf("unexpected payload: %q", f.Payload)
				}
			case <-time.After(time.Second):
				t.Fatal("unrelated stream not delivered")
			}
			select {
			case <-m.Closed():
				t.Fatal("release closed unrelated session")
			default:
			}
		})
	}
}

func TestProductionStreamReleaseSignalsBothSides(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	for _, pair := range []struct {
		name        string
		local, peer *Mux
	}{
		{"client-opens", client, host}, {"host-opens", host, client},
	} {
		t.Run(pair.name, func(t *testing.T) {
			incoming := make(chan Stream, 1)
			pair.peer.OnStream(func(s Stream) { incoming <- s })
			local, err := pair.local.Open(PriBulk, map[string]string{"kind": "test"})
			if err != nil {
				t.Fatal(err)
			}
			var remote Stream
			select {
			case remote = <-incoming:
			case <-time.After(time.Second):
				t.Fatal("peer did not receive OPEN")
			}
			for _, side := range []struct {
				m *Mux
				s Stream
			}{{pair.local, local}, {pair.peer, remote}} {
				side.m.streamMu.Lock()
				st := side.m.streams[side.s.ID]
				side.m.streamMu.Unlock()
				if st == nil || st.done == nil {
					t.Fatal("production stream missing release signal")
				}
				select {
				case <-st.done:
					t.Fatal("signal closed before release")
				default:
				}
				side.m.releaseStream(side.s.ID)
				side.m.releaseStream(side.s.ID)
				select {
				case <-st.done:
				case <-time.After(time.Second):
					t.Fatal("release did not close signal")
				}
				select {
				case <-side.m.Closed():
					t.Fatal("release closed mux")
				default:
				}
			}
		})
	}
}
