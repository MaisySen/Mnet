package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Hello opens the control stream. The client sends it first.
type Hello struct {
	Version  int    `json:"version"`
	NodeName string `json:"nodeName"`
	Want     string `json:"want"` // "screen", "shell", ... informational in v1
}

// HelloReply is the host's answer.
type HelloReply struct {
	Version  int    `json:"version"`
	NodeName string `json:"nodeName"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

// Mux multiplexes typed streams over one net.Conn.
// Frames are written with priority draining; reads dispatch to
// per-stream handlers registered with OnStream.
type Mux struct {
	conn net.Conn
	wmu  sync.Mutex
	// pending[priority] holds queued DATA/OPEN frames per class.
	pending [4][]Frame
	cond    *sync.Cond
	closed  chan struct{}
	closeMu sync.Once

	onStream func(Stream)
	nextID   uint32
	idMu     sync.Mutex
	started  bool // frames queue for the write loop only after start()
}

// Stream is one opened stream, delivered to the registered handler.
type Stream struct {
	Mux      *Mux
	ID       uint32
	OpenJSON json.RawMessage

	in  chan Frame // DATA payloads and FIN
	err error
}

// Dial opens a session to a host and performs the handshake.
// accept is called on the host side (via Listen) to decide consent.
func Dial(conn net.Conn, nodeName string) (*Mux, error) {
	m := newMux(conn, StreamFile)
	h := Hello{Version: protoVersion, NodeName: nodeName, Want: "screen"}
	if err := m.sendJSON(StreamControl, msgOpen, PriInteractive, h); err != nil {
		return nil, err
	}
	f, err := m.readOne()
	if err != nil {
		return nil, err
	}
	if f.StreamID != StreamControl || f.Type != msgData {
		return nil, fmt.Errorf("session: unexpected handshake frame type %d", f.Type)
	}
	var rep HelloReply
	if err := json.Unmarshal(f.Payload, &rep); err != nil {
		return nil, err
	}
	if rep.Version != protoVersion {
		return nil, fmt.Errorf("session: protocol version %d, want %d", rep.Version, protoVersion)
	}
	if !rep.Accepted {
		return nil, fmt.Errorf("session: refused: %s", rep.Reason)
	}
	m.start()
	return m, nil
}

// Listen waits for one session on ln, performs the handshake, and asks
// accept whether to take it. accept may block (e.g. a user prompt).
func Listen(ln net.Listener, nodeName string, accept func(Hello) (bool, string)) (*Mux, error) {
	conn, err := ln.Accept()
	if err != nil {
		return nil, err
	}
	m := newMux(conn, StreamFile)
	f, err := m.readOne()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if f.StreamID != StreamControl || f.Type != msgOpen {
		conn.Close()
		return nil, fmt.Errorf("session: first frame was not a hello")
	}
	var h Hello
	if err := json.Unmarshal(f.Payload, &h); err != nil {
		conn.Close()
		return nil, err
	}
	if h.Version != protoVersion {
		conn.Close()
		return nil, fmt.Errorf("session: protocol version %d, want %d", h.Version, protoVersion)
	}
	ok, reason := accept(h)
	rep := HelloReply{Version: protoVersion, NodeName: nodeName, Accepted: ok, Reason: reason}
	if err := m.sendJSON(StreamControl, msgData, PriInteractive, rep); err != nil {
		conn.Close()
		return nil, err
	}
	if !ok {
		conn.Close()
		return nil, errors.New("session: refused by accept callback")
	}
	m.start()
	return m, nil
}

func newMux(conn net.Conn, firstDynamic uint32) *Mux {
	m := &Mux{conn: conn, closed: make(chan struct{}), nextID: firstDynamic}
	m.cond = sync.NewCond(&m.wmu)
	return m
}

func (m *Mux) start() {
	m.wmu.Lock()
	m.started = true
	m.wmu.Unlock()
	go m.writeLoop()
	go m.readLoop()
}

// OnStream registers the handler for streams opened by the peer.
// OPENs for StreamScreen/StreamInput/StreamChat are dispatched too.
func (m *Mux) OnStream(h func(Stream)) { m.onStream = h }

// Open opens a new dynamic stream (file transfers, future kinds).
func (m *Mux) Open(priority uint8, openPayload any) (Stream, error) {
	m.idMu.Lock()
	id := m.nextID
	m.nextID++
	m.idMu.Unlock()
	s := Stream{Mux: m, ID: id, in: make(chan Frame, 16)}
	var raw json.RawMessage
	if openPayload != nil {
		b, err := json.Marshal(openPayload)
		if err != nil {
			return Stream{}, err
		}
		raw = b
	}
	s.OpenJSON = raw
	if err := m.send(id, msgOpen, priority, raw); err != nil {
		return Stream{}, err
	}
	return s, nil
}

// SendJSON sends a JSON DATA message on a well-known stream.
func (m *Mux) SendJSON(streamID uint32, priority uint8, v any) error {
	return m.sendJSON(streamID, msgData, priority, v)
}

// SendBytes sends a raw DATA message on a stream.
func (m *Mux) SendBytes(streamID uint32, priority uint8, b []byte) error {
	return m.send(streamID, msgData, priority, b)
}

func (s Stream) Recv(timeout time.Duration) (Frame, error) {
	select {
	case f, ok := <-s.in:
		if !ok {
			return Frame{}, io.EOF
		}
		return f, nil
	case <-time.After(timeout):
		return Frame{}, fmt.Errorf("session: recv timeout")
	case <-s.Mux.closed:
		return Frame{}, io.ErrClosedPipe
	}
}

func (m *Mux) Close() error {
	m.closeMu.Do(func() {
		close(m.closed)
		m.cond.Broadcast()
		m.conn.Close()
	})
	return nil
}

func (m *Mux) sendJSON(id uint32, typ byte, pri uint8, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return m.send(id, typ, pri, b)
}

// send queues a frame for priority-drained writing once the mux is
// running; during the handshake (before start) frames go straight to
// the wire under wmu.
func (m *Mux) send(id uint32, typ byte, pri uint8, payload []byte) error {
	f := Frame{StreamID: id, Type: typ, Priority: pri, Payload: payload}
	select {
	case <-m.closed:
		return io.ErrClosedPipe
	default:
	}
	m.wmu.Lock()
	defer m.wmu.Unlock()
	if !m.started {
		return writeFrame(m.conn, f)
	}
	if int(pri) >= len(m.pending) {
		pri = PriBulk
	}
	m.pending[pri] = append(m.pending[pri], f)
	m.cond.Signal()
	return nil
}

func (m *Mux) writeLoop() {
	for {
		m.wmu.Lock()
		for {
			if f, ok := m.popLocked(); ok {
				m.wmu.Unlock()
				if err := writeFrame(m.conn, f); err != nil {
					m.Close()
					return
				}
				m.wmu.Lock()
				continue
			}
			select {
			case <-m.closed:
				m.wmu.Unlock()
				return
			default:
			}
			m.cond.Wait()
		}
	}
}

// popLocked returns the queued frame with the lowest priority class,
// FIFO within a class.
func (m *Mux) popLocked() (Frame, bool) {
	for p := 0; p < len(m.pending); p++ {
		if len(m.pending[p]) > 0 {
			f := m.pending[p][0]
			m.pending[p] = m.pending[p][1:]
			return f, true
		}
	}
	return Frame{}, false
}

func (m *Mux) readOne() (Frame, error) { return readFrame(m.conn) }

func (m *Mux) readLoop() {
	streams := map[uint32]chan Frame{}
	var streamsMu sync.Mutex
	for {
		f, err := readFrame(m.conn)
		if err != nil {
			m.Close()
			return
		}
		switch f.Type {
		case msgPing:
			m.send(f.StreamID, msgPong, PriInteractive, f.Payload)
			continue
		case msgPong:
			continue
		}
		streamsMu.Lock()
		ch, ok := streams[f.StreamID]
		if f.Type == msgOpen {
			ch = make(chan Frame, 16)
			streams[f.StreamID] = ch
			ok = true
			if m.onStream != nil {
				go m.onStream(Stream{Mux: m, ID: f.StreamID, OpenJSON: json.RawMessage(f.Payload), in: ch})
			}
			streamsMu.Unlock()
			continue
		}
		streamsMu.Unlock()
		if !ok {
			continue // DATA for a stream we did not see OPEN: drop
		}
		if f.Type == msgFin || f.Type == msgRst {
			close(ch)
			streamsMu.Lock()
			delete(streams, f.StreamID)
			streamsMu.Unlock()
			continue
		}
		select {
		case ch <- f:
		case <-m.closed:
			return
		}
	}
}
