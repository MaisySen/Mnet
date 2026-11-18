package mobile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fintex.au/mnet/internal/daemon"
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
	// its consent UI here and blocks on the answer. The label has been
	// matched to the encrypted mesh TCP source/current peer map; it is not
	// proof of a Multies person, workspace membership or separate control consent.
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
	access   *activeSessionAccess
}

// SessionViewer is the viewing side of an outgoing session.
type SessionViewer struct {
	viewer *session.Viewer
	mux    *session.Mux
	node   *Node
	access *activeSessionAccess
}

type sessionHostRun struct {
	owner        *Node
	listener     net.Listener
	pending      net.Conn
	active       *SessionHost
	stopped      bool
	done         chan struct{}
	helloTimeout time.Duration
	peerLookup   func() []daemon.SessionPeer
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
	return n.startSessionHostWithPeerLookup(ln, name, cb, helloTimeout, func() []daemon.SessionPeer {
		if n.d == nil {
			return nil
		}
		return n.d.SessionPeers()
	})
}

// The lookup is supplied by the owning daemon in production. Tests inject a
// fixed peer map, never an accept-all fallback when daemon state is missing.
func (n *Node) startSessionHostWithPeerLookup(ln net.Listener, name string, cb HostCallbacks, helloTimeout time.Duration, lookup func() []daemon.SessionPeer) error {
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
	run := &sessionHostRun{owner: n, listener: ln, done: make(chan struct{}), helloTimeout: helloTimeout, peerLookup: lookup}
	hosting = run
	sessMu.Unlock()
	go run.serve(name, cb)
	return nil
}

func (r *sessionHostRun) serve(name string, cb HostCallbacks) {
	defer close(r.done)
	defer r.owner.stopSessionRun(r)
	for {
		var admittedName, admittedKey string
		var admittedSource net.Addr
		m, err := session.Listen(r, name, func(h session.Hello, from net.Addr) (bool, string) {
			sessMu.Lock()
			valid := !r.stopped && hosting == r
			sessMu.Unlock()
			if !valid {
				return false, "host stopped"
			}
			if r.peerLookup == nil {
				return false, "identity check failed"
			}
			verifiedName, verifiedKey, ok := verifiedSessionPeer(r.peerLookup(), h.NodeName, from)
			if !ok {
				return false, "identity check failed"
			}
			// Android's consent UI waits up to60 seconds. Give its reply a
			// separate bounded write window after the initial hello deadline.
			sessMu.Lock()
			conn := r.pending
			sessMu.Unlock()
			if conn == nil || conn.SetDeadline(time.Now().Add(70*time.Second)) != nil {
				return false, "host stopped"
			}
			accepted := cb.AcceptSession(verifiedName)
			sessMu.Lock()
			valid = !r.stopped && hosting == r
			sessMu.Unlock()
			if !valid {
				return false, "host stopped"
			}
			if !accepted {
				return false, "refused"
			}
			// Approval may outlive a map update. Resolve again before admission.
			currentName, currentKey, current := verifiedSessionPeer(r.peerLookup(), h.NodeName, from)
			if !current || currentName != verifiedName || currentKey != verifiedKey {
				return false, "identity check failed"
			}
			admittedName, admittedKey, admittedSource = currentName, currentKey, from
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
		access := &activeSessionAccess{mux: m, current: func() bool {
			sessMu.Lock()
			valid := !r.stopped && hosting == r
			sessMu.Unlock()
			if !valid || r.peerLookup == nil {
				return false
			}
			currentName, currentKey, ok := verifiedSessionPeer(r.peerLookup(), admittedName, admittedSource)
			return ok && currentName == admittedName && currentKey == admittedKey
		}}
		guarded := &authorizedHostCallbacks{HostCallbacks: cb, access: access}
		// Keep the connection reachable by Stop while application geometry
		// initializes, before the active Host exists.
		r.pending = pending
		go access.watch()
		// Geometry is application code. Do not hold the host lifecycle lock
		// while it or any other callback may consult the current authorization.
		sessMu.Unlock()
		h := session.ServeHostWithOptions(m, &appCapturer{cb: guarded}, &appInjector{cb: guarded}, 8, session.HostOptions{
			OnChat: func(cm session.ChatMessage) { b, _ := json.Marshal(cm); guarded.OnChat(string(b)) },
			OnFile: guarded.OnFile,
		})
		sh := &SessionHost{host: h, mux: m, nodeName: name, access: access}
		sessMu.Lock()
		if r.pending == pending {
			r.pending = nil
		}
		if r.stopped || hosting != r {
			sessMu.Unlock()
			h.Stop()
			cb.OnSessionEnd()
			return
		}
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
	if h.access != nil && !h.access.allowed() {
		return errors.New("session authorization ended")
	}
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
	state := n.d.Status()
	if state.Status != "up" {
		return nil, errors.New("mesh is not up")
	}
	requestedIP, err := netip.ParseAddr(peerIP)
	if err != nil || !requestedIP.Is4() {
		return nil, errors.New("session destination must be a mesh IPv4 address")
	}
	source := &net.TCPAddr{IP: net.IP(requestedIP.AsSlice()), Port: SessionPort}
	expectedName, expectedKey, ok := verifiedSessionDestination(n.d.SessionPeers(), source)
	if !ok {
		return nil, errors.New("session destination is not an authorized peer")
	}
	lookup := func() []daemon.SessionPeer {
		peers := n.d.SessionPeers()
		name, key, current := verifiedSessionPeer(peers, expectedName, source)
		if !current || name != expectedName || key != expectedKey {
			return nil
		}
		return peers
	}
	conn, err := dialMeshTCP(state.IPv4, net.JoinHostPort(peerIP, fmt.Sprint(SessionPort)), 10*time.Second)
	if err != nil {
		return nil, err
	}
	return n.connectAuthorizedSession(conn, state.NodeName, cb, 70*time.Second, lookup)
}

// connectAuthorizedSession binds an outgoing viewer to the current mesh peer,
// including an admission check after the host's potentially blocking consent.
func (n *Node) connectAuthorizedSession(conn net.Conn, name string, cb ViewerCallbacks, timeout time.Duration, lookup func() []daemon.SessionPeer) (*SessionViewer, error) {
	if lookup == nil || cb == nil {
		conn.Close()
		return nil, errors.New("viewer authorization unavailable")
	}
	source := conn.RemoteAddr()
	peerName, peerKey, ok := verifiedSessionDestination(lookup(), source)
	if !ok {
		conn.Close()
		return nil, errors.New("session destination is not an authorized peer")
	}
	access := &activeSessionAccess{current: func() bool {
		currentName, currentKey, current := verifiedSessionPeer(lookup(), peerName, source)
		return current && currentName == peerName && currentKey == peerKey
	}}
	guarded := &authorizedViewerCallbacks{ViewerCallbacks: cb, access: access}
	v, err := n.connectSessionWithVerifier(conn, name, guarded, timeout, func(rep session.HelloReply, from net.Addr) error {
		currentName, currentKey, current := verifiedSessionPeer(lookup(), rep.NodeName, from)
		if !current || currentName != peerName || currentKey != peerKey || !access.allowed() {
			return errors.New("session host identity changed or was denied")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Queued frame callbacks may already be running. Attach the mux under the
	// guard's lock and close it if a callback observed denial before attachment.
	access.setMux(v.mux)
	v.access = access
	if !access.allowed() {
		v.Close()
		return nil, errors.New("session host authorization ended")
	}
	go access.watch()
	return v, nil
}

func verifiedSessionDestination(peers []daemon.SessionPeer, from net.Addr) (string, string, bool) {
	tcp, ok := from.(*net.TCPAddr)
	if !ok || tcp == nil {
		return "", "", false
	}
	source, ok := netip.AddrFromSlice(tcp.IP)
	if !ok {
		return "", "", false
	}
	source = source.Unmap()
	claim := ""
	for _, p := range peers {
		ip, err := netip.ParseAddr(p.IPv4)
		if err == nil && ip.Unmap() == source {
			claim = p.Name
			break
		}
	}
	return verifiedSessionPeer(peers, claim, from)
}

func (n *Node) connectSessionWithVerifier(conn net.Conn, name string, cb ViewerCallbacks, timeout time.Duration, verify func(session.HelloReply, net.Addr) error) (*SessionViewer, error) {
	sessMu.Lock()
	if n.sessionClosed || cb == nil || verify == nil {
		sessMu.Unlock()
		conn.Close()
		return nil, errors.New("node closed or viewer callbacks/authorization missing")
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
	m, err := session.DialWithPeerVerifier(conn, name, verify)
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
	if v.access != nil && !v.access.allowed() {
		return errors.New("session authorization ended")
	}
	var ev session.InputEvent
	if err := json.Unmarshal([]byte(evJSON), &ev); err != nil {
		return err
	}
	return v.viewer.SendInput(ev)
}

// SendChat sends a viewer-side chat line.
func (v *SessionViewer) SendChat(text string) error {
	if v.access != nil && !v.access.allowed() {
		return errors.New("session authorization ended")
	}
	return v.viewer.SendChat(v.node.d.Status().NodeName, text)
}

// SendFile pushes one file to the host. Blocks until the ack; call
// from a worker thread.
func (v *SessionViewer) SendFile(path string) error {
	if v.access != nil && !v.access.allowed() {
		return errors.New("session authorization ended")
	}
	return session.SendFile(v.mux, path)
}

// Close ends the session.
func (v *SessionViewer) Close() { v.mux.Close() }

// activeSessionAccess rechecks only the current standalone mesh authorization.
// Once observed invalid it cannot revive, even if a later map restores the key.
// It closes the mux, never Host.Stop, because capture/input may own host workers.
type activeSessionAccess struct {
	mux     *session.Mux
	current func() bool
	revoked atomic.Bool
	muxMu   sync.Mutex
}

func (a *activeSessionAccess) allowed() bool {
	if a.revoked.Load() {
		return false
	}
	if a.current != nil && a.current() && !a.revoked.Load() {
		return true
	}
	a.revoked.Store(true)
	a.muxMu.Lock()
	mux := a.mux
	a.muxMu.Unlock()
	if mux != nil {
		mux.Close()
	}
	return false
}
func (a *activeSessionAccess) setMux(mux *session.Mux) {
	a.muxMu.Lock()
	a.mux = mux
	a.muxMu.Unlock()
	if a.revoked.Load() {
		mux.Close()
	}
}

// watch also checks while a platform capture is blocked or screen output is
// backpressured. This is a local-map check, not a control-plane lease or a
// guarantee of observing an offline administrative change.
func (a *activeSessionAccess) watch() {
	ticker := time.NewTicker(time.Second / 8)
	defer ticker.Stop()
	for {
		if !a.allowed() {
			return
		}
		select {
		case <-a.mux.Closed():
			return
		case <-ticker.C:
		}
	}
}

type authorizedViewerCallbacks struct {
	ViewerCallbacks
	access *activeSessionAccess
}

func (c *authorizedViewerCallbacks) OnFrame(frame []byte) {
	if c.access.allowed() {
		c.ViewerCallbacks.OnFrame(frame)
	}
}
func (c *authorizedViewerCallbacks) OnChat(message string) {
	if c.access.allowed() {
		c.ViewerCallbacks.OnChat(message)
	}
}

type authorizedHostCallbacks struct {
	HostCallbacks
	access *activeSessionAccess
}

func (c *authorizedHostCallbacks) Capture() ([]byte, error) {
	if !c.access.allowed() {
		return nil, errors.New("session authorization ended")
	}
	frame, err := c.HostCallbacks.Capture()
	// Do not publish a frame whose platform capture outlived authorization.
	if !c.access.allowed() {
		return nil, errors.New("session authorization ended")
	}
	return frame, err
}
func (c *authorizedHostCallbacks) Inject(event string) {
	if c.access.allowed() {
		c.HostCallbacks.Inject(event)
	}
}
func (c *authorizedHostCallbacks) OnChat(message string) {
	if c.access.allowed() {
		c.HostCallbacks.OnChat(message)
	}
}
func (c *authorizedHostCallbacks) OnFile(name string, size int64) {
	if c.access.allowed() {
		c.HostCallbacks.OnFile(name, size)
	}
}

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

// Select this node's assigned source explicitly. Android can otherwise choose
// an underlay source for sockets opened by the VPN app itself.
func dialMeshTCP(self, target string, timeout time.Duration) (net.Conn, error) {
	ip := net.ParseIP(self)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() {
		return nil, errors.New("assigned mesh IPv4 required")
	}
	dialer := net.Dialer{Timeout: timeout, LocalAddr: &net.TCPAddr{IP: ip}}
	return dialer.Dial("tcp4", target)
}

// verifiedSessionPeer authenticates only a current mesh-map label, not a Multies
// person. The TCP source belongs to the encrypted mesh; a claimed hello label
// alone must never choose who the host sees in a consent prompt.
func verifiedSessionPeer(peers []daemon.SessionPeer, claim string, from net.Addr) (string, string, bool) {
	tcp, ok := from.(*net.TCPAddr)
	if !ok || tcp == nil {
		return "", "", false
	}
	source, ok := netip.AddrFromSlice(tcp.IP)
	if !ok {
		return "", "", false
	}
	source = source.Unmap()
	if claim == "" || strings.TrimSpace(claim) != claim {
		return "", "", false
	}
	result := ""
	key := ""
	sourceMatches, nameMatches := 0, 0
	for _, peer := range peers {
		ip, err := netip.ParseAddr(peer.IPv4)
		sameSource := err == nil && ip.Unmap() == source
		sameName := strings.EqualFold(peer.Name, claim)
		if sameSource {
			sourceMatches++
		}
		if sameName {
			nameMatches++
		}
		if sameSource && sameName {
			result = peer.Name
			key = peer.PublicKey
		}
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(key)
	validKey := err == nil && len(raw) == 32 && base64.StdEncoding.EncodeToString(raw) == key
	return result, key, result != "" && sourceMatches == 1 && nameMatches == 1 && validKey
}
