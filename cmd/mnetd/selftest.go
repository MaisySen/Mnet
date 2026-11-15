package main

// Self-test mode for validation rigs: run the daemon on an in-memory
// TUN instead of a kernel one, push one ICMP ping at a peer, and report
// whether an encrypted packet came back through the mesh. This needs no
// root and no /dev/net/tun, so it works in sandboxes and CI.

import (
	"fmt"
	"log"
	"net/netip"
	"time"

	"golang.zx2c4.com/wireguard/tun/tuntest"

	"fintex.au/mnet/internal/daemon"
)

// selfTestDaemon builds a daemon on a ChannelTUN. The returned channel
// gets a copy of every packet the mesh delivers inbound.
func selfTestDaemon(stateDir string, verbose bool) (*daemon.Daemon, *tuntest.ChannelTUN, chan []byte, error) {
	ct := tuntest.NewChannelTUN()
	got := make(chan []byte, 16)
	go func() {
		for pkt := range ct.Inbound {
			cp := make([]byte, len(pkt))
			copy(cp, pkt)
			select {
			case got <- cp:
			default:
			}
		}
	}()
	d, err := daemon.New(daemon.Config{
		StateDir:     stateDir,
		Verbose:      verbose,
		TUN:          ct.TUN(),
		ManagedIface: false,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return d, ct, got, nil
}

// runSelfTest brings the node up, pings peerIP, waits for any inbound
// packet (the peer's ping), then exits. Both sides run it; each proves
// it received the other through the mesh.
func runSelfTest(d *daemon.Daemon, ct *tuntest.ChannelTUN, got chan []byte, server, network, name, authKey, peerIP string) error {
	if err := d.Up(server, network, name, authKey); err != nil {
		return err
	}
	st := d.Status()
	if st.IPv4 == "" {
		return fmt.Errorf("up but no mesh address")
	}
	dst, err := netip.ParseAddr(peerIP)
	if err != nil {
		return err
	}
	src, err := netip.ParseAddr(st.IPv4)
	if err != nil {
		return err
	}
	log.Printf("mnetd self-test: mesh ip %s, pinging %s", st.IPv4, peerIP)

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case ct.Outbound <- tuntest.Ping(dst, src):
		default:
		}
		select {
		case pkt := <-got:
			log.Printf("mnetd self-test: PASS, got %d byte packet from the mesh", len(pkt))
			// Stay up: the other side still needs us in the mesh. The
			// validation script kills the process once both sides pass.
			for {
				time.Sleep(time.Hour)
			}
		case <-time.After(1500 * time.Millisecond):
		}
	}
	return fmt.Errorf("no packet from the mesh within 60s")
}
