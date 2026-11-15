package session

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
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
	conn        net.Conn
	traceMu     sync.Mutex
	traceEvents [512]frameEvent
	traceNext   uint64
	wmu         sync.Mutex
	// pending[priority] holds queued DATA/OPEN frames per class.
	pending     [4][]Frame
	queuedBytes int
	drainSlot   int
	cond        *sync.Cond
	closed      chan struct{}
	closeMu     sync.Once

	lastRead   atomic.Int64 // unixnano; any inbound frame proves liveness
	lastRTT    atomic.Int64 // nanoseconds; last keepalive round trip
	onStreamMu sync.RWMutex
	onStream   func(Stream)
	nextID     uint32 // dynamic stream ids, partitioned by side
	idMu       sync.Mutex
	started    bool                    // frames queue for the write loop only after start()
	opened     map[uint32]bool         // streams this side has OPENed
	streams    map[uint32]*streamState // inbound delivery, both directions
	streamMu   sync.Mutex
}

// streamState is one known stream. Streams opened by the peer before
// an OnStream handler exists wait in handled=false state; OnStream
// dispatches them when it is registered.
type streamState struct {
	ch      chan Frame
	open    json.RawMessage
	handled bool
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
	m := newMux(conn, StreamFile) // client dynamic ids: 4, 6, 8, ...
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
func Listen(ln net.Listener, nodeName string, accept func(Hello, net.Addr) (bool, string)) (*Mux, error) {
	conn, err := ln.Accept()
	if err != nil {
		return nil, err
	}
	m := newMux(conn, StreamFile+1) // host dynamic ids: 5, 7, 9, ...
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
	ok, reason := accept(h, conn.RemoteAddr())
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
	m := &Mux{conn: conn, closed: make(chan struct{}), nextID: firstDynamic,
		opened: map[uint32]bool{StreamControl: true}, streams: map[uint32]*streamState{}}
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
// Safe to call after the session has started; streams that arrived
// earlier without a handler are dispatched now.
func (m *Mux) OnStream(h func(Stream)) {
	m.onStreamMu.Lock()
	m.onStream = h
	m.onStreamMu.Unlock()
	m.streamMu.Lock()
	var pending []Stream
	for id, st := range m.streams {
		if !st.handled {
			st.handled = true
			pending = append(pending, Stream{Mux: m, ID: id, OpenJSON: st.open, in: st.ch})
		}
	}
	m.streamMu.Unlock()
	for _, s := range pending {
		go h(s)
	}
}

func (m *Mux) streamHandler() func(Stream) {
	m.onStreamMu.RLock()
	defer m.onStreamMu.RUnlock()
	return m.onStream
}

// Open opens a new dynamic stream (file transfers, future kinds).
// Client and host allocate from disjoint id ranges so either side can
// open without coordination.
func (m *Mux) Open(priority uint8, openPayload any) (Stream, error) {
	m.idMu.Lock()
	id := m.nextID
	m.nextID += 2
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
	m.streamMu.Lock()
	if len(m.streams) >= maxActiveStreams {
		m.streamMu.Unlock()
		return Stream{}, fmt.Errorf("session: active stream limit reached")
	}
	m.streams[id] = &streamState{ch: s.in, open: raw, handled: true}
	m.streamMu.Unlock()
	if err := m.send(id, msgOpen, priority, raw); err != nil {
		m.releaseStream(id)
		return Stream{}, err
	}
	m.wmu.Lock()
	m.opened[id] = true
	m.wmu.Unlock()
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

// Recv returns the next DATA frame on the stream. A timeout <= 0
// blocks until a frame, FIN/RST, or mux close.
func (s Stream) Recv(timeout time.Duration) (Frame, error) {
	if timeout <= 0 {
		select {
		case f, ok := <-s.in:
			if !ok {
				return Frame{}, io.EOF
			}
			if f.Type == msgRst {
				return Frame{}, ErrStreamReset
			}
			return f, nil
		case <-s.Mux.closed:
			return Frame{}, io.ErrClosedPipe
		}
	}
	select {
	case f, ok := <-s.in:
		if !ok {
			return Frame{}, io.EOF
		}
		if f.Type == msgRst {
			return Frame{}, ErrStreamReset
		}
		return f, nil
	case <-time.After(timeout):
		return Frame{}, fmt.Errorf("session: recv timeout")
	case <-s.Mux.closed:
		return Frame{}, io.ErrClosedPipe
	}
}

// Closed returns a channel closed when the session ends.
func (m *Mux) Closed() <-chan struct{} { return m.closed }

// Keepalive pings the peer every interval and closes the mux when no
// frame has arrived for timeout. NAT reboots, sleep-wake cycles and
// dead relays otherwise leave a session looking open forever. Both
// sides run it; the ping payload echoes nothing meaningful.
func (m *Mux) Keepalive(interval, timeout time.Duration) {
	m.lastRead.Store(time.Now().UnixNano())
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-m.closed:
				return
			case now := <-t.C:
				if now.UnixNano()-m.lastRead.Load() > timeout.Nanoseconds() {
					log.Printf("session: no traffic for %s, closing", timeout)
					m.Close()
					return
				}
				var ts [8]byte
				binary.BigEndian.PutUint64(ts[:], uint64(now.UnixNano()))
				if err := m.send(StreamControl, msgPing, PriInteractive, ts[:]); err != nil {
					return
				}
			}
		}
	}()
}

// PeerAddr returns the remote address of the underlying connection,
// for hosts that bind the claimed peer name to the source mesh IP.
func (m *Mux) PeerAddr() net.Addr { return m.conn.RemoteAddr() }

// RTT is the last measured keepalive round trip; zero before the
// first pong. Hosts use it to adapt stream quality to the path.
func (m *Mux) RTT() time.Duration { return time.Duration(m.lastRTT.Load()) }

func (m *Mux) Close() error {
	m.closeMu.Do(func() {
		m.dumpTrace()
		// Interrupt any synchronous handshake write before taking wmu.
		m.conn.Close()
		m.wmu.Lock()
		close(m.closed)
		m.cond.Broadcast()
		m.wmu.Unlock()
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
	if len(payload) > maxPayload {
		return fmt.Errorf("session: payload %d exceeds %d", len(payload), maxPayload)
	}
	// send is asynchronous after start. The caller may reuse its buffer
	// as soon as we return (notably SendFile's chunk buffer).
	f := Frame{StreamID: id, Type: typ, Priority: pri}
	select {
	case <-m.closed:
		return io.ErrClosedPipe
	default:
	}
	m.wmu.Lock()
	defer m.wmu.Unlock()
	select {
	case <-m.closed:
		return io.ErrClosedPipe
	default:
	}
	if !m.started {
		f.Payload = payload
		return writeFrame(m.conn, f)
	}
	if int(pri) >= len(m.pending) {
		pri = PriBulk
	}
	needed := 0
	// Backpressure bounds memory for slow viewers or large transfers.
	for {
		needed = headerLen + len(payload)
		if typ == msgData && !m.opened[id] {
			needed += headerLen + 2
		}
		if m.queuedBytes+needed <= maxQueuedBytes {
			break
		}
		select {
		case <-m.closed:
			return io.ErrClosedPipe
		default:
		}
		m.cond.Wait()
	}
	select {
	case <-m.closed:
		return io.ErrClosedPipe
	default:
	}
	f.Priority = pri
	f.Payload = append([]byte(nil), payload...)
	m.queuedBytes += needed
	// Implicit OPEN on first send: OPEN is how the peer learns the
	// stream exists, so DATA on a fresh stream is preceded by one.
	if typ == msgData && !m.opened[id] {
		m.opened[id] = true
		m.pending[pri] = append(m.pending[pri], Frame{StreamID: id, Type: msgOpen, Priority: pri, Payload: []byte(`{}`)})
	}
	m.pending[pri] = append(m.pending[pri], f)
	m.traceFrame("queued", f)
	m.cond.Signal()
	return nil
}

func (m *Mux) writeLoop() {
	for {
		m.wmu.Lock()
		for {
			if f, ok := m.popLocked(); ok {
				m.wmu.Unlock()
				m.traceFrame("write-start", f)
				if err := writeFrame(m.conn, f); err != nil {
					m.Close()
					return
				}
				m.traceFrame("write-done", f)
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

// maxQueuedBytes bounds queued wire frames, excluding one in-flight write.
const maxQueuedBytes = 16 << 20

// Weighted round robin favors input and screen traffic without letting a
// continuous screen stream starve chat or file completion indefinitely.
var drainOrder = [...]int{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 3}

// popLocked preserves FIFO within each class and bounds starvation.
func (m *Mux) popLocked() (Frame, bool) {
	for i := 0; i < len(drainOrder); i++ {
		p := drainOrder[m.drainSlot]
		m.drainSlot = (m.drainSlot + 1) % len(drainOrder)
		if len(m.pending[p]) == 0 {
			continue
		}
		f := m.pending[p][0]
		m.pending[p][0] = Frame{}
		m.pending[p] = m.pending[p][1:]
		m.queuedBytes -= headerLen + len(f.Payload)
		m.cond.Broadcast()
		return f, true
	}
	return Frame{}, false
}

// Metadata-only tracing is opt-in for diagnosing stalled sessions. It never
// logs payloads, peer credentials, file names or file contents.
var traceFrames = os.Getenv("MNET_SESSION_TRACE") == "1"

type frameEvent struct {
	at    time.Time
	stage string
	id    uint32
	typ   byte
	size  int
}

func (m *Mux) traceFrame(stage string, f Frame) {
	if !traceFrames {
		return
	}
	m.traceMu.Lock()
	m.traceEvents[m.traceNext%uint64(len(m.traceEvents))] = frameEvent{time.Now(), stage, f.StreamID, f.Type, len(f.Payload)}
	m.traceNext++
	m.traceMu.Unlock()
}

func (m *Mux) dumpTrace() {
	if !traceFrames {
		return
	}
	m.traceMu.Lock()
	defer m.traceMu.Unlock()
	start := uint64(0)
	if m.traceNext > uint64(len(m.traceEvents)) {
		start = m.traceNext - uint64(len(m.traceEvents))
	}
	for i := start; i < m.traceNext; i++ {
		e := m.traceEvents[i%uint64(len(m.traceEvents))]
		log.Printf("session trace mux=%p at=%s stage=%s stream=%d type=%d bytes=%d", m, e.at.Format(time.RFC3339Nano), e.stage, e.id, e.typ, e.size)
	}
}

func (m *Mux) readOne() (Frame, error) { return readFrame(m.conn) }

func (m *Mux) readLoop() {
	for {
		f, err := readFrame(m.conn)
		if err != nil {
			m.Close()
			return
		}
		m.traceFrame("read", f)
		m.lastRead.Store(time.Now().UnixNano())
		switch f.Type {
		case msgPing:
			m.send(f.StreamID, msgPong, PriInteractive, f.Payload)
			continue
		case msgPong:
			if len(f.Payload) == 8 {
				sent := int64(binary.BigEndian.Uint64(f.Payload))
				m.lastRTT.Store(time.Now().UnixNano() - sent)
			}
			continue
		}
		m.streamMu.Lock()
		st, ok := m.streams[f.StreamID]
		if f.Type == msgOpen {
			if !ok {
				if len(m.streams) >= maxActiveStreams {
					m.streamMu.Unlock()
					m.Close()
					return
				}
				st = &streamState{ch: make(chan Frame, 16), open: json.RawMessage(f.Payload)}
				if h := m.streamHandler(); h != nil {
					st.handled = true
					m.streams[f.StreamID] = st
					go h(Stream{Mux: m, ID: f.StreamID, OpenJSON: st.open, in: st.ch})
				} else {
					m.streams[f.StreamID] = st
				}
			}
			// An OPEN for a stream we opened locally is the peer's
			// implicit-open echo on reply; the channel already exists.
			m.streamMu.Unlock()
			continue
		}
		m.streamMu.Unlock()
		if !ok {
			continue // DATA for a stream we did not see OPEN: drop
		}
		if f.Type == msgFin || f.Type == msgRst {
			if f.Type == msgRst {
				select {
				case st.ch <- f:
				case <-m.closed:
					return
				}
			}
			close(st.ch)
			m.streamMu.Lock()
			delete(m.streams, f.StreamID)
			m.streamMu.Unlock()
			continue
		}
		select {
		case st.ch <- f:
		case <-m.closed:
			return
		}
	}
}

// maxActiveStreams bounds stream tables and inbound per-stream queues.
// Well-known channels count too; crossing the inbound cap ends the session.
const maxActiveStreams = 256

// releaseStream forgets a completed dynamic stream. It does not close
// the channel: readLoop owns channel delivery/closing and may be in flight.
func (m *Mux) releaseStream(id uint32) {
	m.streamMu.Lock()
	delete(m.streams, id)
	m.streamMu.Unlock()
	m.wmu.Lock()
	delete(m.opened, id)
	m.wmu.Unlock()
}

// ErrStreamReset distinguishes peer cancellation from a complete FIN.
var ErrStreamReset = errors.New("session: stream reset by peer")
