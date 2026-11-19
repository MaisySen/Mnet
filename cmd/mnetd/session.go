package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

const sessionPort = 5909

// allowedViewer applies the -session-allow list. An empty list allows
// every verified mesh peer.
func allowedViewer(allow map[string]bool, name string) bool {
	if len(allow) == 0 {
		return true
	}
	for a := range allow {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

// sessionAudit is one line of the host's session log.
type sessionAudit struct {
	At       time.Time `json:"at"`
	Event    string    `json:"event"` // accept, refuse, end
	Viewer   string    `json:"viewer"`
	SourceIP string    `json:"source_ip"`
	Reason   string    `json:"reason,omitempty"`
	Duration string    `json:"duration,omitempty"`
}

type sessionLogger struct {
	mu   sync.Mutex
	path string
}

func (l *sessionLogger) log(ev sessionAudit) {
	ev.At = time.Now().UTC()
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("mnetd: session audit: %v", err)
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// Headless authorization is immediate. Bound untrusted hello/reply I/O so a
// silent socket cannot occupy the listener loop forever.
type sessionHelloListener struct {
	net.Listener
	budget   time.Duration
	accepted *net.Conn
}

func (listener sessionHelloListener) Accept() (net.Conn, error) {
	conn, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(listener.budget)); err != nil {
		conn.Close()
		return nil, err
	}
	*listener.accepted = conn
	return conn, nil
}

func listenHeadlessSession(listener net.Listener, name string, budget time.Duration, accept func(session.Hello, net.Addr) (bool, string)) (*session.Mux, error) {
	return listenSessionWithApproval(listener, name, budget, func(h session.Hello, from net.Addr, _ net.Conn) (bool, string) { return accept(h, from) })
}
func listenSessionWithApproval(listener net.Listener, name string, budget time.Duration, accept func(session.Hello, net.Addr, net.Conn) (bool, string)) (*session.Mux, error) {
	var conn net.Conn
	mux, err := session.Listen(sessionHelloListener{listener, budget, &conn}, name, func(h session.Hello, from net.Addr) (bool, string) { return accept(h, from, conn) })
	if err != nil {
		return nil, err
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		mux.Close()
		return nil, err
	}
	return mux, nil
}

// runSessionHost waits for the mesh to come up, then hosts remote
// sessions on the node's mesh IP. autoAccept is required: a headless
// daemon cannot prompt, so hosting without it refuses every session
// with a reason the viewer can show. Every request is bound to its
// source mesh IP, filtered by the allow list, and recorded in
// <state>/sessions.jsonl.
func runSessionHost(owner *sessionHostOwner, d *daemon.Daemon, stateDir string, autoAccept bool, allow map[string]bool, recordDir string) {
	runSessionHostWithUI(owner, d, stateDir, autoAccept, allow, recordDir, nil)
}

func runSessionHostWithUI(owner *sessionHostOwner, d *daemon.Daemon, stateDir string, autoAccept bool, allow map[string]bool, recordDir string, ui attendedUI) {
	var attendedDone <-chan struct{}
	defer func() {
		owner.Close()
		if attendedDone != nil {
			<-attendedDone
		}
	}()
	var attendedBusy atomic.Bool
	audit := &sessionLogger{path: filepath.Join(stateDir, "sessions.jsonl")}
	var ip string
	for {
		st := d.Status()
		if st.Status == "up" && st.IPv4 != "" {
			ip = st.IPv4
			break
		}
		select {
		case <-owner.ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	addr := net.JoinHostPort(ip, fmt.Sprint(sessionPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("mnetd: session host: listen %s: %v", addr, err)
		return
	}
	if !owner.attachListener(ln) {
		return
	}
	defer ln.Close()
	log.Printf("mnetd: session host listening on %s", addr)
	nodeName := d.Status().NodeName
	for {
		start := time.Now()
		var viewer, viewerIP, refuseWhy string
		admittedAttended := false
		negotiateControl := false
		budget := 15 * time.Second
		var admittedKey string
		var admittedSource net.Addr
		m, err := listenSessionWithApproval(ownedSessionListener{ln, owner}, nodeName, budget, func(h session.Hello, from net.Addr, conn net.Conn) (bool, string) {
			viewer = h.NodeName
			refuseWhy = ""
			if from != nil {
				viewerIP, _, _ = net.SplitHostPort(from.String())
			}
			if !autoAccept && ui == nil {
				refuseWhy = "host runs headless; restart mnetd with -session-auto-accept"
				return false, refuseWhy
			}
			canonical, key, ok := daemon.MatchSessionPeer(d.SessionPeers(), h.NodeName, from)
			if !ok {
				refuseWhy = "current peer identity unavailable or ambiguous"
				return false, "identity check failed"
			}
			viewer, admittedKey, admittedSource = canonical, key, from
			if !allowedViewer(allow, h.NodeName) {
				refuseWhy = "not on the allow list"
				return false, "node is not on this host's allow list"
			}
			if ui != nil {
				if !attendedBusy.CompareAndSwap(false, true) {
					refuseWhy = "attended desktop already has a session"
					return false, refuseWhy
				}
				admittedAttended = true
				// Only an authenticated, allow-listed peer gets time for a local answer.
				currentPeer := func() bool {
					name, key, ok := daemon.MatchSessionPeer(d.SessionPeers(), viewer, admittedSource)
					return ok && name == viewer && key == admittedKey && allowedViewer(allow, viewer)
				}
				if !approveAttendedConnection(owner.ctx, ui, viewer, conn, currentPeer) {
					refuseWhy = "local viewing approval unavailable, declined or retired"
					return false, refuseWhy
				}
				negotiateControl = h.ControlV1
			}
			return true, ""
		})
		if err != nil {
			if admittedAttended {
				attendedBusy.Store(false)
			}
			select {
			case <-owner.ctx.Done():
				return
			default:
			}
			log.Printf("mnetd: session host: %v", err)
			if viewer != "" && refuseWhy != "" {
				audit.log(sessionAudit{Event: "refuse", Viewer: viewer, SourceIP: viewerIP, Reason: refuseWhy})
			}
			continue
		}
		authorized := func() bool {
			currentName, currentKey, ok := daemon.MatchSessionPeer(d.SessionPeers(), viewer, admittedSource)
			return ok && currentName == viewer && currentKey == admittedKey && allowedViewer(allow, viewer)
		}
		if !authorized() {
			if admittedAttended {
				attendedBusy.Store(false)
			}
			m.Close()
			continue
		}
		cap, capErr := newCapturer(stateDir)
		if capErr != nil {
			log.Printf("mnetd: session host: capture unavailable: %v", capErr)
			if admittedAttended {
				attendedBusy.Store(false)
			}
			m.Close()
			continue
		}
		if ui != nil {
			_, attendedDone = serveAttendedSession(owner.ctx, m, viewer, negotiateControl, ui, cap, newAttendedInjector, authorized)
			go func(done <-chan struct{}, viewer, source string, start time.Time) {
				<-done
				attendedBusy.Store(false)
				audit.log(sessionAudit{Event: "attended-end", Viewer: viewer, SourceIP: source, Duration: time.Since(start).Round(time.Second).String()})
			}(attendedDone, viewer, viewerIP, start)
			audit.log(sessionAudit{Event: "attended-view", Viewer: viewer, SourceIP: viewerIP})
			continue
		}
		inj, injErr := newInjector()
		if injErr != nil {
			log.Printf("mnetd: session host: input injection unavailable: %v", injErr)
		}
		var rec *session.Recorder
		recPath := ""
		if recordDir != "" {
			os.MkdirAll(recordDir, 0o700)
			recPath = filepath.Join(recordDir, fmt.Sprintf("%s-%s.mjpeg", viewer, start.UTC().Format("20060102-150405")))
			if r, err := session.NewRecorder(recPath); err == nil {
				rec = r
			} else {
				log.Printf("mnetd: session recording unavailable: %v", err)
				recPath = ""
			}
		}
		h := session.ServeHostWithOptions(m, cap, inj, 8, session.HostOptions{
			FileDir: filepath.Join(stateDir, "received-files"), Recorder: rec, Authorized: authorized,
		})
		log.Printf("mnetd: session started for %s (%s)", viewer, viewerIP)
		audit.log(sessionAudit{Event: "accept", Viewer: viewer, SourceIP: viewerIP, Reason: recPath})
		go func(viewer, viewerIP string, start time.Time, rec *session.Recorder, recPath string) {
			<-m.Closed()
			h.Stop()
			if rec != nil {
				if err := rec.Close(); err != nil {
					log.Printf("mnetd: session recording incomplete: %v", err)
				}
				log.Printf("mnetd: session recording %s: %s", recPath, rec)
			}
			dur := time.Since(start).Round(time.Second)
			log.Printf("mnetd: session with %s ended after %s", viewer, dur)
			audit.log(sessionAudit{Event: "end", Viewer: viewer, SourceIP: viewerIP, Duration: dur.String(), Reason: recPath})
		}(viewer, viewerIP, start, rec, recPath)
	}
}
