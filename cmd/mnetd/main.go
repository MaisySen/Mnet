// mnetd is the Mnet node daemon. Run it as root (Linux, macOS) or
// Administrator (Windows), then drive it with the mnet CLI.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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
		stateDir = flag.String("state", defaultStateDir(), "state directory")
		ifname   = flag.String("ifname", "mnet0", "TUN interface name")
		verbose  = flag.Bool("verbose", false, "verbose wireguard logging")
		showVer  = flag.Bool("version", false, "print version and exit")
		up       = flag.Bool("up", false, "join immediately using saved state or the flags below")
		server   = flag.String("server", "", "control plane URL, needed on first join")
		network  = flag.String("network", "", "network to join, needed on first join")
		name     = flag.String("name", "", "node name (default: hostname)")
		authKey  = flag.String("auth-key", "", "pre-auth key for instant join")
	)
	flag.Parse()
	if *showVer {
		log.Printf("mnetd %s", version.Version)
		return
	}

	d, err := daemon.New(daemon.Config{StateDir: *stateDir, IfName: *ifname, Verbose: *verbose})
	if err != nil {
		log.Fatalf("mnetd: %v", err)
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
	log.Printf("mnetd %s ready (state: %s)", version.Version, *stateDir)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("mnetd: shutting down")
	d.Down()
}
