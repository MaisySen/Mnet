// mnetd is the Mnet node daemon. Run it as root (Linux, macOS) or
// Administrator (Windows), then drive it with the mnet CLI.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.zx2c4.com/wireguard/tun/tuntest"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/version"
)

func defaultStateDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "mnet")
	}
	return "/var/lib/mnet"
}

func main() {
	var (
		stateDir  = flag.String("state", defaultStateDir(), "state directory")
		ifname    = flag.String("ifname", "mnet0", "TUN interface name")
		verbose   = flag.Bool("verbose", false, "verbose wireguard logging")
		showVer   = flag.Bool("version", false, "print version and exit")
		up        = flag.Bool("up", false, "join immediately using saved state or the flags below")
		fakeTun   = flag.Bool("fake-tun", false, "validation rigs: in-memory TUN, no root needed")
		selfTest  = flag.String("self-test", "", "validation rigs: ping this peer mesh IP and exit")
		server    = flag.String("server", "", "control plane URL, needed on first join")
		network   = flag.String("network", "", "network to join, needed on first join")
		name      = flag.String("name", "", "node name (default: hostname)")
		authKey   = flag.String("auth-key", "", "pre-auth key for instant join")
		advExit   = flag.Bool("advertise-exit-node", false, "offer to carry other nodes' internet traffic")
		advRoutes = flag.String("advertise-routes", "", "comma separated subnet CIDRs this node serves")
		useExit   = flag.String("exit-node", "", "peer name or mesh IP to send internet traffic through")
		sessHost  = flag.Bool("session-host", false, "host remote-desktop sessions on this node (mesh port 5909)")
		sessAuto  = flag.Bool("session-auto-accept", false, "accept sessions without prompting (required on headless daemons)")
		sessAllow = flag.String("session-allow", "", "comma separated node names allowed to start sessions (default: every mesh peer)")
		sessProof = flag.Bool("session-proof", false, "validation rigs: run a full two-node session over the mesh on userspace tunnels and exit")
		proofDir  = flag.String("proof-dir", "/tmp/mnet-session-proof", "artifact directory for -session-proof")
	)
	flag.Parse()
	if *showVer {
		log.Printf("mnetd %s", version.Version)
		return
	}
	if *sessProof {
		if err := runSessionProof(*server, *network, *authKey, *proofDir, *verbose); err != nil {
			log.Fatalf("mnetd session-proof: %v", err)
		}
		return
	}

	var d *daemon.Daemon
	var err error
	var got chan []byte
	var ct *tuntest.ChannelTUN
	if *fakeTun {
		d, ct, got, err = selfTestDaemon(*stateDir, *verbose)
	} else {
		d, err = daemon.New(daemon.Config{StateDir: *stateDir, IfName: *ifname, Verbose: *verbose, ManagedIface: true})
	}
	if err != nil {
		log.Fatalf("mnetd: %v", err)
	}

	if *selfTest != "" {
		if err := runSelfTest(d, ct, got, *server, *network, *name, *authKey, *selfTest); err != nil {
			log.Fatalf("mnetd self-test: %v", err)
		}
		return
	}

	if *advExit || *advRoutes != "" || *useExit != "" {
		var routes []string
		if *advRoutes != "" {
			routes = strings.Split(*advRoutes, ",")
		}
		if err := d.SetRouting(*advExit, routes, *useExit); err != nil {
			log.Printf("mnetd: routing: %v", err)
		}
	}
	if *up {
		go func() {
			if err := d.Up(*server, *network, *name, *authKey); err != nil {
				log.Printf("mnetd: up: %v", err)
			}
		}()
	}

	go func() {
		if err := d.ServeLocalAPI(); err != nil {
			log.Fatalf("mnetd: local api: %v", err)
		}
	}()
	if *sessHost {
		allow := map[string]bool{}
		for _, n := range strings.Split(*sessAllow, ",") {
			if n = strings.TrimSpace(n); n != "" {
				allow[n] = true
			}
		}
		go runSessionHost(d, *stateDir, *sessAuto, allow)
	}
	log.Printf("mnetd %s ready (state: %s)", version.Version, *stateDir)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("mnetd: shutting down")
	d.Down()
}
