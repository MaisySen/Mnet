package mobile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

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

var (
	sessMu     sync.Mutex
	hostListen net.Listener
	hostActive *SessionHost
)

// StartSessionHost begins accepting session requests on the node's
// mesh IP. One session at a time; a second caller is refused while one
// is active. cb.AcceptSession decides consent.
func (n *Node) StartSessionHost(cb HostCallbacks) error {
	ip := n.d.Status().IPv4
	if ip == "" {
		return errors.New("not up")
	}
	sessMu.Lock()
	defer sessMu.Unlock()
	if hostListen != nil {
		return errors.New("session host already running")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, fmt.Sprint(SessionPort)))
	if err != nil {
		return err
	}
	hostListen = ln
	go func() {
		for {
			m, err := session.Listen(ln, n.d.Status().NodeName, func(h session.Hello, _ net.Addr) (bool, string) {
				sessMu.Lock()
				busy := hostActive != nil
				sessMu.Unlock()
				if busy {
					return false, "another session is active"
				}
				if !cb.AcceptSession(h.NodeName) {
					return false, "refused"
				}
				return true, ""
			})
			if err != nil {
				return // listener closed by StopSessionHost
			}
			cap := &appCapturer{cb: cb}
			inj := &appInjector{cb: cb}

			h := session.ServeHostWithOptions(m, cap, inj, 8, session.HostOptions{
				OnChat: func(cm session.ChatMessage) { b, _ := json.Marshal(cm); cb.OnChat(string(b)) },
				OnFile: cb.OnFile,
			})
			sh := &SessionHost{host: h, mux: m, nodeName: n.d.Status().NodeName}
			sessMu.Lock()
			hostActive = sh
			sessMu.Unlock()
			<-m.Closed()
			h.Stop()
			cb.OnSessionEnd()
			sessMu.Lock()
			hostActive = nil
			sessMu.Unlock()
		}
	}()
	return nil
}

// StopSessionHost ends any active session and stops accepting.
func (n *Node) StopSessionHost() {
	sessMu.Lock()
	defer sessMu.Unlock()
	if hostActive != nil {
		hostActive.host.Stop()
		hostActive = nil
	}
	if hostListen != nil {
		hostListen.Close()
		hostListen = nil
	}
}

// SendChat sends a host-side chat line.
func (h *SessionHost) SendChat(text string) error {
	return h.host.SendChat(h.nodeName, text)
}

// ConnectSession dials a peer's session host by mesh IP.
func (n *Node) ConnectSession(peerIP string, cb ViewerCallbacks) (*SessionViewer, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(peerIP, fmt.Sprint(SessionPort)), 10*1e9)
	if err != nil {
		return nil, err
	}
	m, err := session.Dial(conn, n.d.Status().NodeName)
	if err != nil {
		conn.Close()
		return nil, err
	}

	v := session.ConnectViewerWithOptions(m, session.ViewerOptions{
		OnFrame: func(fr session.ScreenFrame) { cb.OnFrame(fr.Image) },
		OnChat:  func(cm session.ChatMessage) { b, _ := json.Marshal(cm); cb.OnChat(string(b)) },
	})
	go func() {
		<-m.Closed()
		cb.OnClosed()
	}()
	return &SessionViewer{viewer: v, mux: m, node: n}, nil
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
