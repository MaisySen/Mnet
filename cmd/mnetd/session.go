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
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

const sessionPort = 5909

// bindViewer verifies the viewer's claimed node name against the
// control-plane-issued netmap: the connection's source mesh IP must
// be the address the control plane assigned to that name. Without
// this, any mesh peer could claim any name in its Hello.
func bindViewer(peers []daemon.PeerStatus, name, sourceIP string) error {
	for _, p := range peers {
		if strings.EqualFold(p.Name, name) {
			if p.IPv4 != sourceIP {
				return fmt.Errorf("name %q belongs to %s, not %s", name, p.IPv4, sourceIP)
			}
			return nil
		}
	}
	return fmt.Errorf("no peer named %q in the network map", name)
}

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

// runSessionHost waits for the mesh to come up, then hosts remote
// sessions on the node's mesh IP. autoAccept is required: a headless
// daemon cannot prompt, so hosting without it refuses every session
// with a reason the viewer can show. Every request is bound to its
// source mesh IP, filtered by the allow list, and recorded in
// <state>/sessions.jsonl.
func runSessionHost(d *daemon.Daemon, stateDir string, autoAccept bool, allow map[string]bool) {
	audit := &sessionLogger{path: filepath.Join(stateDir, "sessions.jsonl")}
	var ip string
	for {
		st := d.Status()
		if st.Status == "up" && st.IPv4 != "" {
			ip = st.IPv4
			break
		}
		time.Sleep(time.Second)
	}
	addr := net.JoinHostPort(ip, fmt.Sprint(sessionPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("mnetd: session host: listen %s: %v", addr, err)
		return
	}
	log.Printf("mnetd: session host listening on %s", addr)
	nodeName := d.Status().NodeName
	for {
		start := time.Now()
		var viewer, viewerIP, refuseWhy string
		m, err := session.Listen(ln, nodeName, func(h session.Hello, from net.Addr) (bool, string) {
			viewer = h.NodeName
			refuseWhy = ""
			if from != nil {
				viewerIP, _, _ = net.SplitHostPort(from.String())
			}
			if !autoAccept {
				refuseWhy = "host runs headless; restart mnetd with -session-auto-accept"
				return false, refuseWhy
			}
			if err := bindViewer(d.Status().Peers, h.NodeName, viewerIP); err != nil {
				refuseWhy = err.Error()
				return false, "identity check failed"
			}
			if !allowedViewer(allow, h.NodeName) {
				refuseWhy = "not on the allow list"
				return false, "node is not on this host's allow list"
			}
			return true, ""
		})
		if err != nil {
			log.Printf("mnetd: session host: %v", err)
			if viewer != "" && refuseWhy != "" {
				audit.log(sessionAudit{Event: "refuse", Viewer: viewer, SourceIP: viewerIP, Reason: refuseWhy})
			}
			continue
		}
		cap, capErr := newCapturer()
		if capErr != nil {
			log.Printf("mnetd: session host: capture unavailable: %v", capErr)
			m.Close()
			continue
		}
		inj, injErr := newInjector()
		if injErr != nil {
			log.Printf("mnetd: session host: input injection unavailable: %v", injErr)
		}
		h := session.ServeHost(m, cap, inj, 8)
		log.Printf("mnetd: session started for %s (%s)", viewer, viewerIP)
		audit.log(sessionAudit{Event: "accept", Viewer: viewer, SourceIP: viewerIP})
		go func(viewer, viewerIP string, start time.Time) {
			<-m.Closed()
			h.Stop()
			dur := time.Since(start).Round(time.Second)
			log.Printf("mnetd: session with %s ended after %s", viewer, dur)
			audit.log(sessionAudit{Event: "end", Viewer: viewer, SourceIP: viewerIP, Duration: dur.String()})
		}(viewer, viewerIP, start)
	}
}
