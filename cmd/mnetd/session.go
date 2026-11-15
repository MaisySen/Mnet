package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

const sessionPort = 5909

// runSessionHost waits for the mesh to come up, then hosts remote
// sessions on the node's mesh IP. autoAccept is required: a headless
// daemon cannot prompt, so hosting without it refuses every session
// with a reason the viewer can show.
func runSessionHost(d *daemon.Daemon, autoAccept bool) {
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
		m, err := session.Listen(ln, nodeName, func(h session.Hello) (bool, string) {
			if !autoAccept {
				return false, "host runs headless; restart mnetd with -session-auto-accept"
			}
			return true, ""
		})
		if err != nil {
			log.Printf("mnetd: session host: %v", err)
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
		log.Printf("mnetd: session started")
		go func() {
			<-m.Closed()
			h.Stop()
			log.Printf("mnetd: session ended after %s", time.Since(start).Round(time.Second))
		}()
	}
}
