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
	RequireAttendedView bool   `json:"require_attended_view,omitempty"`
	Version             int    `json:"version"`
	NodeName            string `json:"nodeName"`
	ControlV1           bool   `json:"control_v1,omitempty"`
	Want                string `json:"want"` // "screen", "shell", ... informational in v1
}

// HelloReply is the host's answer.
type HelloReply struct {
	AttendedView bool   `json:"attended_view,omitempty"`
	Version      int    `json:"version"`
	NodeName     string `json:"nodeName"`
	Accepted     bool   `json:"accepted"`
	Reason       string `json:"reason,omitempty"`
}

// Mux multiplexes typed streams over one net.Conn.
// Frames are written with priority draining; reads dispatch to
// per-stream handlers registered with OnStream.
type Mux struct {
	conn         net.Conn
	traceEnabled bool // fixed before loops start; never a process-wide test toggle
	traceMu      sync.Mutex
	traceEvents  [512]frameEvent
	traceNext    uint64
	wmu          sync.Mutex
	// pending[priority] holds queued DATA/OPEN frames per class.
	pending     [4][]Frame
	queuedBytes int
	drainSlot   int
	cond        *sync.Cond
	closed      chan struct{}
	closeMu     sync.Once

	inboundBytes atomic.Int64 // queued DATA payloads across all streams
	lastRead     atomic.Int64 // unixnano; any inbound frame proves liveness
	lastRTT      atomic.Int64 // nanoseconds; last keepalive round trip
	onStreamMu   sync.RWMutex
	onStream     func(Stream)
	nextID       uint32 // dynamic stream ids, partitioned by side
	idMu         sync.Mutex
	started      bool                    // frames queue for the write loop only after start()
	opened       map[uint32]bool         // streams this side has OPENed
	streams      map[uint32]*streamState // inbound delivery, both directions
	streamMu     sync.Mutex
}

// streamState is one known stream. Streams opened by the peer before
// an OnStream handler exists wait in handled=false state; OnStream
// dispatches them when it is registered.
type streamState struct {
	accountMu   sync.Mutex
	queuedBytes int64
	released    bool
	ch          chan Frame
	peerReset   chan struct{} // peer cancellation, including after FIN
	peerFIN     chan struct{}
	remoteFin   bool          // read-loop-owned receive half-close; owner still owes release
	done        chan struct{} // closes on local release to unblock read-loop delivery
	open        json.RawMessage
	handled     bool
}

// Stream is one opened stream, delivered to the registered handler.
type Stream struct {
	Mux      *Mux
	ID       uint32
	OpenJSON json.RawMessage

	state *streamState
	in    chan Frame // DATA payloads and FIN
	err   error
}

// Dial opens a session to a host and performs the handshake.
// Ownership of conn transfers to Dial: errors close it; success transfers it
// to the returned mux. The caller must set any handshake deadline beforehand.
// accept is called on the host side (via Listen) to decide consent.
func Dial(conn net.Conn, nodeName string) (*Mux, error) {
	return DialWithPeerVerifier(conn, nodeName, nil)
}

// DialWithPeerVerifier checks the host reply before starting stream dispatch.
// The callback must bind the reply to a trusted source, not trust its label.
// A rejection closes the connection and publishes no active mux.
func DialWithPeerVerifier(conn net.Conn, nodeName string, verify func(HelloReply, net.Addr) error) (*Mux, error) {
	return DialWithControl(conn, nodeName, verify, false)
}

// DialWithControl explicitly opts into attended control negotiation. Legacy
// callers do not advertise a capability they cannot enforce.
func DialWithControl(conn net.Conn, nodeName string, verify func(HelloReply, net.Addr) error, control bool) (*Mux, error) {
	return dialCapabilities(conn, nodeName, verify, control, false)
}

// DialAttendedView requires an explicit attended-view reply. Older/headless
// listeners cannot silently substitute their legacy acceptance policy.
func DialAttendedView(conn net.Conn, nodeName string, verify func(HelloReply, net.Addr) error) (*Mux, error) {
	return dialCapabilities(conn, nodeName, verify, false, true)
}
func dialCapabilities(conn net.Conn, nodeName string, verify func(HelloReply, net.Addr) error, control, attended bool) (*Mux, error) {
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()
	m := newMux(conn, StreamFile) // client dynamic ids: 4, 6, 8, ...
	h := Hello{Version: protoVersion, NodeName: nodeName, Want: "screen", ControlV1: control, RequireAttendedView: attended}
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
	if attended && !rep.AttendedView {
		return nil, errors.New("session: host does not support required attended viewing")
	}
	if verify != nil {
		if err := verify(rep, conn.RemoteAddr()); err != nil {
			return nil, err
		}
	}
	m.start()
	success = true
	return m, nil
}

// Listen waits for one session on ln, performs the handshake, and asks
// accept whether to take it. accept may block (e.g. a user prompt).
func Listen(ln net.Listener, nodeName string, accept func(Hello, net.Addr) (bool, string)) (*Mux, error) {
	return listenCapabilities(ln, nodeName, accept, false)
}

// ListenAttended is only for a host whose callback performs local view
// approval. The marker describes this path, not external human authority.
func ListenAttended(ln net.Listener, nodeName string, accept func(Hello, net.Addr) (bool, string)) (*Mux, error) {
	return listenCapabilities(ln, nodeName, accept, true)
}
func listenCapabilities(ln net.Listener, nodeName string, accept func(Hello, net.Addr) (bool, string), attended bool) (*Mux, error) {
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
	ok, reason := false, "attended viewing requires a local approval host"
	if !h.RequireAttendedView || attended {
		ok, reason = accept(h, conn.RemoteAddr())
	}
	rep := HelloReply{Version: protoVersion, NodeName: nodeName, Accepted: ok, Reason: reason, AttendedView: attended && ok}
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
	m := &Mux{conn: conn, traceEnabled: os.Getenv("MNET_SESSION_TRACE") == "1", closed: make(chan struct{}), nextID: firstDynamic,
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
	select {
	case <-m.closed:
		m.streamMu.Unlock()
		return
	default:
	}
	var pending []Stream
	for id, st := range m.streams {
		if !st.handled {
			st.handled = true
			pending = append(pending, Stream{Mux: m, ID: id, OpenJSON: st.open, in: st.ch, state: st})
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
	return m.open(priority, openPayload, true)
}
func (m *Mux) open(priority uint8, openPayload any, wait bool) (Stream, error) {
	var raw json.RawMessage
	if openPayload != nil {
		b, err := json.Marshal(openPayload)
		if err != nil {
			return Stream{}, err
		}
		raw = b
	}
	if len(raw) > maxOpenPayload {
		return Stream{}, fmt.Errorf("session: OPEN metadata exceeds policy")
	}
	m.idMu.Lock()
	id := m.nextID
	m.nextID += 2
	m.idMu.Unlock()
	s := Stream{Mux: m, ID: id, in: make(chan Frame, 16)}
	s.OpenJSON = raw
	m.streamMu.Lock()
	select {
	case <-m.closed:
		m.streamMu.Unlock()
		return Stream{}, io.ErrClosedPipe
	default:
	}
	if len(m.streams) >= maxActiveStreams {
		m.streamMu.Unlock()
		return Stream{}, fmt.Errorf("session: active stream limit reached")
	}
	s.state = &streamState{ch: s.in, done: make(chan struct{}), peerReset: make(chan struct{}), peerFIN: make(chan struct{}), open: raw, handled: true}
	m.streams[id] = s.state
	m.streamMu.Unlock()
	if err := m.sendWithWait(id, msgOpen, priority, raw, wait); err != nil {
		m.releaseStream(id)
		return Stream{}, err
	}
	m.wmu.Lock()
	select {
	case <-m.closed:
		m.wmu.Unlock()
		m.releaseStream(id)
		return Stream{}, io.ErrClosedPipe
	default:
		m.opened[id] = true
	}
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
// blocks until a frame, FIN/RST, local release or mux close. Known terminal
// state wins over queued frames/FIN; already-returned work is not revoked.
func (s Stream) Recv(timeout time.Duration) (Frame, error) {
	var released, reset <-chan struct{}
	if s.state != nil {
		released = s.state.done
		reset = s.state.peerReset
	}
	terminal := func() bool {
		select {
		case <-s.Mux.closed:
			return true
		case <-released:
			return true
		default:
			return false
		}
	}
	select {
	case <-reset:
		return Frame{}, ErrStreamReset
	default:
	}
	if terminal() {
		return Frame{}, io.ErrClosedPipe
	}
	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}
	select {
	case f, ok := <-s.in:
		if ok {
			s.Mux.releaseInbound(&f)
		}
		// A ready frame/FIN and terminal signal may be selected together.
		// Return its budget, but never dispatch known retired-session work.
		select {
		case <-reset:
			return Frame{}, ErrStreamReset
		default:
		}
		if terminal() {
			return Frame{}, io.ErrClosedPipe
		}
		if !ok {
			return Frame{}, io.EOF
		}
		if f.Type == msgRst {
			return Frame{}, ErrStreamReset
		}
		return f, nil
	case <-deadline:
		if terminal() {
			return Frame{}, io.ErrClosedPipe
		}
		return Frame{}, fmt.Errorf("session: recv timeout")
	case <-reset:
		return Frame{}, ErrStreamReset
	case <-released:
		return Frame{}, io.ErrClosedPipe
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
				// A full application queue must not park the watchdog inside
				// send: it still owes the next inbound-liveness deadline check.
				// Skip only this probe when capacity is unavailable.
				if err := m.sendWithWait(StreamControl, msgPing, PriInteractive, ts[:], false); err != nil {
					select {
					case <-m.closed:
						return
					default:
					}
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
		// A retained closed mux must not retain queued file/screen contents.
		// One frame already handed to the socket writer is outside this queue.
		for i := range m.pending {
			m.pending[i] = nil
		}
		m.queuedBytes = 0
		clear(m.opened)
		m.cond.Broadcast()
		m.wmu.Unlock()
		// Retire active inbound ownership too. Existing Stream handles may
		// retain their channels, but can no longer dispatch from this mux.
		m.streamMu.Lock()
		for id, st := range m.streams {
			m.releaseStreamBudget(st)
			delete(m.streams, id)
		}
		m.streamMu.Unlock()
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
	return m.sendWithWait(id, typ, pri, payload, true)
}

// controlJSON never waits on a slow peer. Callers must fail closed on error.
func (m *Mux) controlJSON(id uint32, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return m.sendWithWait(id, msgData, PriInteractive, b, false)
}
func (m *Mux) sendWithWait(id uint32, typ byte, pri uint8, payload []byte, wait bool) error {
	return m.sendWithCancel(id, typ, pri, payload, wait, nil)
}

// sendWithCancel lets one transfer stop waiting for shared outbound capacity
// without closing unrelated channels. The owner must broadcast cond on cancel.
func (m *Mux) sendWithCancel(id uint32, typ byte, pri uint8, payload []byte, wait bool, canceled <-chan struct{}) error {
	select {
	case <-canceled:
		return ErrStreamReset
	default:
	}
	if len(payload) > maxPayload {
		return fmt.Errorf("session: payload %d exceeds %d", len(payload), maxPayload)
	}
	// send is asynchronous after start. The caller may reuse its buffer
	// as soon as we return (notably SendFile's chunk buffer).
	f := Frame{StreamID: id, Type: typ, Priority: pri}
	select {
	case <-canceled:
		return ErrStreamReset
	case <-m.closed:
		return io.ErrClosedPipe
	default:
	}
	m.wmu.Lock()
	defer m.wmu.Unlock()
	select {
	case <-canceled:
		return ErrStreamReset
	case <-m.closed:
		return io.ErrClosedPipe
	default:
	}
	if !m.started {
		if !wait {
			return errors.New("session: control requires running mux")
		}
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
		case <-canceled:
			return ErrStreamReset
		case <-m.closed:
			return io.ErrClosedPipe
		default:
		}
		if !wait {
			return errors.New("session: outbound control capacity exhausted")
		}
		m.cond.Wait()
	}
	select {
	case <-canceled:
		return ErrStreamReset
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

// tryReset queues best-effort stream termination without waiting for outbound
// capacity. Cleanup must not depend on a peer draining its receive socket.
// False means no reset was queued; it does not close the shared session.
func (m *Mux) tryReset(id uint32) bool {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	select {
	case <-m.closed:
		return false
	default:
	}
	if !m.started || m.queuedBytes+headerLen > maxQueuedBytes {
		return false
	}
	f := Frame{StreamID: id, Type: msgRst, Priority: PriBulk}
	m.pending[PriBulk] = append(m.pending[PriBulk], f)
	m.queuedBytes += headerLen
	m.traceFrame("queued", f)
	m.cond.Signal()
	return true
}

func (m *Mux) writeLoop() {
	for {
		m.wmu.Lock()
		for {
			select {
			case <-m.closed:
				m.wmu.Unlock()
				return
			default:
			}
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
// The setting is captured for each new Mux, before its goroutines start.

type frameEvent struct {
	at    time.Time
	stage string
	id    uint32
	typ   byte
	size  int
}

func (m *Mux) traceFrame(stage string, f Frame) {
	if !m.traceEnabled {
		return
	}
	m.traceMu.Lock()
	m.traceEvents[m.traceNext%uint64(len(m.traceEvents))] = frameEvent{time.Now(), stage, f.StreamID, f.Type, len(f.Payload)}
	m.traceNext++
	m.traceMu.Unlock()
}

func (m *Mux) dumpTrace() {
	if !m.traceEnabled {
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
		select {
		case <-m.closed:
			m.streamMu.Unlock()
			return
		default:
		}
		st, ok := m.streams[f.StreamID]
		if f.Type == msgOpen {
			if !ok {
				if len(m.streams) >= maxActiveStreams {
					m.streamMu.Unlock()
					m.Close()
					return
				}
				st = &streamState{ch: make(chan Frame, 16), done: make(chan struct{}), peerReset: make(chan struct{}), peerFIN: make(chan struct{}), open: json.RawMessage(f.Payload)}
				if h := m.streamHandler(); h != nil {
					st.handled = true
					m.streams[f.StreamID] = st
					go h(Stream{Mux: m, ID: f.StreamID, OpenJSON: st.open, in: st.ch, state: st})
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

		if f.Type == msgRst {
			// A reset remains observable after a peer FIN while local final
			// processing is active. readLoop is the sole channel closer.
			if st.peerReset != nil {
				close(st.peerReset)
			}
			if !st.remoteFin {
				close(st.ch)
				st.remoteFin = true
			}
			m.streamMu.Lock()
			if m.streams[f.StreamID] == st {
				delete(m.streams, f.StreamID)
			}
			m.streamMu.Unlock()
			continue
		}
		if f.Type == msgFin {
			if !st.remoteFin {
				st.remoteFin = true
				if st.peerFIN != nil {
					close(st.peerFIN)
				}
				close(st.ch)
			}
			// FIN ends receiving bytes, not owner finalization. Retain this
			// bounded entry until release so a later reset can reach its owner.
			continue
		}
		if st.remoteFin {
			continue
		} // never send DATA into a half-closed channel
		// Bound retained wire DATA across streams, not only each channel.
		// Slow consumers must not turn 256 independent queues into GiBs.
		st.accountMu.Lock()
		if st.released {
			st.accountMu.Unlock()
			continue
		}
		f.queueOwner = st
		f.queueCost = int64(len(f.Payload))
		st.queuedBytes += f.queueCost
		over := m.inboundBytes.Add(f.queueCost) > maxInboundBytes
		st.accountMu.Unlock()
		if over {
			m.releaseInbound(&f)
			m.Close()
			return
		}
		select {
		case st.ch <- f:
		case <-st.done:
			m.releaseInbound(&f)
		case <-m.closed:
			m.releaseInbound(&f)
			return
		}
	}
}

// This bounds queued DATA only. OPEN metadata, one read in flight, decoded
// images and application-owned copies have separate limits/lifetimes.
const maxInboundBytes int64 = 16 << 20

func (m *Mux) releaseInbound(f *Frame) {
	if f.queueCost > 0 && f.queueOwner != nil {
		st := f.queueOwner
		st.accountMu.Lock()
		if !st.released {
			st.queuedBytes -= f.queueCost
			m.inboundBytes.Add(-f.queueCost)
		}
		st.accountMu.Unlock()
	}
	f.queueCost = 0
	f.queueOwner = nil
}

// maxActiveStreams bounds stream tables and inbound per-stream queues.
// Well-known channels count too; crossing the inbound cap ends the session.
const maxActiveStreams = 256

// releaseStream forgets a completed dynamic stream and unblocks any read-loop
// delivery into its queue. Only readLoop closes the frame channel.
func (m *Mux) releaseStream(id uint32) {
	m.streamMu.Lock()
	if st, ok := m.streams[id]; ok {
		m.releaseStreamBudget(st)
	}
	delete(m.streams, id)
	m.streamMu.Unlock()
	m.wmu.Lock()
	delete(m.opened, id)
	m.wmu.Unlock()
}

// ErrStreamReset distinguishes peer cancellation from a complete FIN.
var ErrStreamReset = errors.New("session: stream reset by peer")

// release also returns charges when FIN already removed the stream table entry.
func (s Stream) release() {
	s.Mux.releaseStream(s.ID)
	if s.state != nil {
		s.Mux.releaseStreamBudget(s.state)
	}
}
func (m *Mux) releaseStreamBudget(st *streamState) {
	st.accountMu.Lock()
	defer st.accountMu.Unlock()
	if !st.released {
		st.released = true
		if st.done != nil {
			close(st.done)
		}
	}
	m.inboundBytes.Add(-st.queuedBytes)
	st.queuedBytes = 0
}

// peerResetSignal remains live through FIN until the local owner releases work.
func (s Stream) peerResetSignal() <-chan struct{} {
	if s.state == nil {
		return nil
	}
	return s.state.peerReset
}
