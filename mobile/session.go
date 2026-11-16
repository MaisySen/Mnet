package mobile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"fintex.au/mnet/internal/session"
)

// Session support for the apps: the engine runs the protocol, the app
// supplies screen frames and input injection through callbacks because
// both live in platform APIs (MediaProjection, AccessibilityService).
//
// The session listens and dials on the node's mesh IP: packets for a
// phone's own mesh address are delivered to the local stack from the
// tunnel, so no extra plumbing is needed.

// SessionPort is the mesh port sessions live on.
const SessionPort = 5909

// HostCallbacks is implemented by the app hosting a session.
type HostCallbacks interface {
	// Capture returns one JPEG frame, or nil when the screen is
	// unchanged since the last call.
	Capture() ([]byte, error)
	// ScreenWidth and ScreenHeight return the capture dimensions in
	// pixels. (Two methods because gomobile callbacks return one value.)
	ScreenWidth() int
	ScreenHeight() int
	// Inject applies one input event, JSON-encoded session.InputEvent.
	Inject(evJSON string)
	// AcceptSession decides whether an incoming session from the named
	// peer node may start. Called on a worker goroutine; the app shows
	// its consent UI here and blocks on the answer.
	AcceptSession(peerNodeName string) bool
	// OnChat delivers one incoming chat message, JSON-encoded.
	OnChat(msgJSON string)
	// OnFile reports a finished incoming file transfer.
	OnFile(name string, size int64)
	// OnSessionEnd fires when the session closes.
	OnSessionEnd()
}

// ViewerCallbacks is implemented by the app viewing a session.
type ViewerCallbacks interface {
	// OnFrame delivers one screen update's JPEG bytes.
	OnFrame(jpeg []byte)
	// OnChat delivers one incoming chat message, JSON-encoded.
	OnChat(msgJSON string)
	// OnClosed fires when the session ends.
	OnClosed()
}

// SessionHost is the hosting side of one incoming session.
type SessionHost struct {
	host     *session.Host
	mux      *session.Mux
	nodeName string
}

// SessionViewer is the viewing side of an outgoing session.
type SessionViewer struct {
	viewer *session.Viewer
	mux    *session.Mux
	node   *Node
}

type sessionHostRun struct {
	owner        *Node
	listener     net.Listener
	pending      net.Conn
	active       *SessionHost
	stopped      bool
	done         chan struct{}
	helloTimeout time.Duration
}

var (
	sessMu  sync.Mutex
	hosting *sessionHostRun
)

// Accept tracks a connection even while the peer handshake or consent blocks.
// Closing only the listener would leave that accepted connection alive.
func (r *sessionHostRun) Accept() (net.Conn, error) {
	conn, err := r.listener.Accept()
	if err != nil {
		return nil, err
	}
	sessMu.Lock()
	defer sessMu.Unlock()
	if r.stopped {
		conn.Close()
		return nil, net.ErrClosed
	}
	if err := conn.SetDeadline(time.Now().Add(r.helloTimeout)); err != nil {
		conn.Close()
		return nil, err
	}
	r.pending = conn
	return conn, nil
}
func (r *sessionHostRun) Close() error   { return r.listener.Close() }
func (r *sessionHostRun) Addr() net.Addr { return r.listener.Addr() }

// StartSessionHost begins accepting on the node's mesh IP. Only this node
// can stop its host; stale workers/consent cannot replace a newer host.
func (n *Node) StartSessionHost(cb HostCallbacks) error {
	ip := n.d.Status().IPv4
	if ip == "" {
		return errors.New("not up")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, fmt.Sprint(SessionPort)))
	if err != nil {
		return err
	}
	return n.startSessionHost(ln, n.d.Status().NodeName, cb)
}

func (n *Node) startSessionHost(ln net.Listener, name string, cb HostCallbacks) error {
	return n.startSessionHostTimeout(ln, name, cb, 15*time.Second)
}
func (n *Node) startSessionHostTimeout(ln net.Listener, name string, cb HostCallbacks, helloTimeout time.Duration) error {
	if cb == nil {
		ln.Close()
		return errors.New("session callbacks required")
	}
	sessMu.Lock()
	if n.sessionClosed {
		sessMu.Unlock()
		ln.Close()
		return errors.New("node is closed")
	}
	if hosting != nil {
		sessMu.Unlock()
		ln.Close()
		return errors.New("session host already running")
	}
	run := &sessionHostRun{owner: n, listener: ln, done: make(chan struct{}), helloTimeout: helloTimeout}
	hosting = run
	sessMu.Unlock()
	go run.serve(name, cb)
	return nil
}

func (r *sessionHostRun) serve(name string, cb HostCallbacks) {
	defer close(r.done)
	defer r.owner.stopSessionRun(r)
	for {
		m, err := session.Listen(r, name, func(h session.Hello, _ net.Addr) (bool, string) {
			sessMu.Lock()
			valid := !r.stopped && hosting == r
			sessMu.Unlock()
			if !valid {
				return false, "host stopped"
			}
			// Android's consent UI waits up to60 seconds. Give its reply a
			// separate bounded write window after the initial hello deadline.
			sessMu.Lock()
			conn := r.pending
			sessMu.Unlock()
			if conn == nil || conn.SetDeadline(time.Now().Add(70*time.Second)) != nil {
				return false, "host stopped"
			}
			accepted := cb.AcceptSession(h.NodeName)
			sessMu.Lock()
			valid = !r.stopped && hosting == r
			sessMu.Unlock()
			if !valid {
				return false, "host stopped"
			}
			if !accepted {
				return false, "refused"
			}
			return true, ""
		})
		sessMu.Lock()
		pending := r.pending
		r.pending = nil
		if err != nil {
			// A rejected/malformed accepted peer is not a listener failure.
			// Keep serving after closing its connection, unless stopped.
			retry := pending != nil && !r.stopped && hosting == r
			sessMu.Unlock()
			if pending != nil {
				pending.Close()
			}
			if retry {
				continue
			}
			return
		}
		if r.stopped || hosting != r {
			sessMu.Unlock()
			m.Close()
			return
		}
		if err := pending.SetDeadline(time.Time{}); err != nil {
			sessMu.Unlock()
			m.Close()
			continue
		}
		h := session.ServeHostWithOptions(m, &appCapturer{cb: cb}, &appInjector{cb: cb}, 8, session.HostOptions{
			OnChat: func(cm session.ChatMessage) { b, _ := json.Marshal(cm); cb.OnChat(string(b)) },
			OnFile: cb.OnFile,
		})
		sh := &SessionHost{host: h, mux: m, nodeName: name}
		r.active = sh
		sessMu.Unlock()
		<-m.Closed()
		h.Stop()
		cb.OnSessionEnd()
		sessMu.Lock()
		if r.active == sh {
			r.active = nil
		}
		stopped := r.stopped
		sessMu.Unlock()
		if stopped {
			return
		}
	}
}

// StopSessionHost closes only the host this node owns. No callback/worker wait
// happens under sessMu, since application callbacks also use shared state.
func (n *Node) StopSessionHost() {
	sessMu.Lock()
	r := hosting
	sessMu.Unlock()
	if r != nil && r.owner == n {
		n.stopSessionRun(r)
	}
}
func (n *Node) stopSessionRun(r *sessionHostRun) {
	sessMu.Lock()
	if r.owner != n || r.stopped {
		sessMu.Unlock()
		return
	}
	r.stopped = true
	if hosting == r {
		hosting = nil
	}
	pending, active := r.pending, r.active
	r.pending = nil
	r.active = nil
	sessMu.Unlock()
	r.listener.Close()
	if pending != nil {
		pending.Close()
	}
	if active != nil {
		active.host.Stop()
	}
}

// SendChat sends a host-side chat line.
func (h *SessionHost) SendChat(text string) error {
	return h.host.SendChat(h.nodeName, text)
}

// ConnectSession dials a peer's session host by mesh IP.
func (n *Node) ConnectSession(peerIP string, cb ViewerCallbacks) (*SessionViewer, error) {
	sessMu.Lock()
	closed := n.sessionClosed
	sessMu.Unlock()
	if closed {
		return nil, errors.New("node is closed")
	}
	if cb == nil {
		return nil, errors.New("viewer callbacks required")
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(peerIP, fmt.Sprint(SessionPort)), 10*time.Second)
	if err != nil {
		return nil, err
	}
	return n.connectSession(conn, n.d.Status().NodeName, cb, 70*time.Second)
}

func (n *Node) connectSession(conn net.Conn, name string, cb ViewerCallbacks, timeout time.Duration) (*SessionViewer, error) {
	sessMu.Lock()
	if n.sessionClosed || cb == nil {
		sessMu.Unlock()
		conn.Close()
		return nil, errors.New("node closed or viewer callbacks missing")
	}
	if n.viewers == nil {
		n.viewers = make(map[net.Conn]struct{})
	}
	n.viewers[conn] = struct{}{}
	sessMu.Unlock()
	failed := true
	defer func() {
		if failed {
			n.releaseViewer(conn)
			conn.Close()
		}
	}()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	m, err := session.Dial(conn, name)
	if err != nil {
		return nil, err
	}
	// Stop removes ownership before closing connections. Check again after a
	// successful handshake so it cannot publish a late viewer after Stop/Close.
	sessMu.Lock()
	_, owned := n.viewers[conn]
	sessMu.Unlock()
	if !owned {
		m.Close()
		return nil, errors.New("viewer stopped")
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		m.Close()
		return nil, err
	}
	v := session.ConnectViewerWithOptions(m, session.ViewerOptions{
		OnFrame: func(fr session.ScreenFrame) { cb.OnFrame(fr.Image) },
		OnChat:  func(cm session.ChatMessage) { b, _ := json.Marshal(cm); cb.OnChat(string(b)) },
	})
	failed = false
	go func() { <-m.Closed(); n.releaseViewer(conn); cb.OnClosed() }()
	return &SessionViewer{viewer: v, mux: m, node: n}, nil
}

func (n *Node) releaseViewer(conn net.Conn) { sessMu.Lock(); delete(n.viewers, conn); sessMu.Unlock() }
func (n *Node) stopViewers(permanent bool) {
	sessMu.Lock()
	if permanent {
		n.sessionClosed = true
	}
	connections := n.viewers
	n.viewers = nil
	sessMu.Unlock()
	for conn := range connections {
		conn.Close()
	}
}

// SendInputJSON sends one input event, JSON-encoded session.InputEvent.
func (v *SessionViewer) SendInputJSON(evJSON string) error {
	var ev session.InputEvent
	if err := json.Unmarshal([]byte(evJSON), &ev); err != nil {
		return err
	}
	return v.viewer.SendInput(ev)
}

// SendChat sends a viewer-side chat line.
func (v *SessionViewer) SendChat(text string) error {
	return v.viewer.SendChat(v.node.d.Status().NodeName, text)
}

// SendFile pushes one file to the host. Blocks until the ack; call
// from a worker thread.
func (v *SessionViewer) SendFile(path string) error {
	return session.SendFile(v.mux, path)
}

// Close ends the session.
func (v *SessionViewer) Close() { v.mux.Close() }

// appCapturer adapts HostCallbacks to session.Capturer.
type appCapturer struct {
	cb    HostCallbacks
	first bool
}

func (a *appCapturer) Capture() ([]byte, bool, error) {
	img, err := a.cb.Capture()
	if err != nil || img == nil {
		return img, false, err
	}
	return img, true, nil
}

func (a *appCapturer) Geometry() (int, int) { return a.cb.ScreenWidth(), a.cb.ScreenHeight() }

// appInjector adapts HostCallbacks to session.Injector.
type appInjector struct{ cb HostCallbacks }

func (a *appInjector) Inject(ev session.InputEvent) error {
	b, _ := json.Marshal(ev)
	a.cb.Inject(string(b))
	return nil
}
