// mnetd is the Mnet node daemon. Run it as root (Linux, macOS) or
// Administrator (Windows), then drive it with the mnet CLI.
// On Windows it also installs and runs as a service: see
// service_windows.go.
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

// options carries the parsed flags so the same boot path serves the
// console and the Windows service handler.
type options struct {
	stateDir string
	ifname   string
	verbose  bool
	up       bool
	fakeTun  bool
	selfTest string
	server   string
	network  string
	name     string
	authKey  string

	advExit   bool
	advRoutes string
	useExit   string

	sessHost  bool
	sessAuto  bool
	sessAllow string
	sessRec   string
	sessProof bool
	proofDir  string

	serviceAction string
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
		sessRec   = flag.String("session-record", "", "record hosted sessions as MJPEG videos in this directory")
		sessProof = flag.Bool("session-proof", false, "validation rigs: run a full two-node session over the mesh on userspace tunnels and exit")
		proofDir  = flag.String("proof-dir", "/tmp/mnet-session-proof", "artifact directory for -session-proof")
		service   = flag.String("service", "", "Windows service control: install, remove, run")
	)
	flag.Parse()
	if *showVer {
		log.Printf("mnetd %s", version.Version)
		return
	}
	o := options{
		stateDir:      *stateDir,
		ifname:        *ifname,
		verbose:       *verbose,
		up:            *up,
		fakeTun:       *fakeTun,
		selfTest:      *selfTest,
		server:        *server,
		network:       *network,
		name:          *name,
		authKey:       *authKey,
		advExit:       *advExit,
		advRoutes:     *advRoutes,
		useExit:       *useExit,
		sessHost:      *sessHost,
		sessAuto:      *sessAuto,
		sessAllow:     *sessAllow,
		sessRec:       *sessRec,
		sessProof:     *sessProof,
		proofDir:      *proofDir,
		serviceAction: *service,
	}
	if err := dispatch(o); err != nil {
		log.Fatalf("mnetd: %v", err)
	}
}

// dispatch routes the run: explicit service control, an implicit
// Windows service session, or the console daemon.
func dispatch(o options) error {
	if o.serviceAction != "" {
		return runServiceAction(o)
	}
	if isServiceSession() {
		return runAsService(o)
	}
	return runDaemon(o, nil)
}

// runDaemon boots the daemon and blocks until SIGINT/SIGTERM or, when
// stop is non-nil (the Windows service manager), until stop closes.
func runDaemon(o options, stop <-chan struct{}) error {
	if o.sessProof {
		if err := runSessionProof(o.server, o.network, o.authKey, o.proofDir, o.verbose); err != nil {
			return err
		}
		return nil
	}

	var d *daemon.Daemon
	var err error
	var got chan []byte
	var ct *tuntest.ChannelTUN
	if o.fakeTun {
		d, ct, got, err = selfTestDaemon(o.stateDir, o.verbose)
	} else {
		d, err = daemon.New(daemon.Config{StateDir: o.stateDir, IfName: o.ifname, Verbose: o.verbose, ManagedIface: true})
	}
	if err != nil {
		return err
	}

	if o.selfTest != "" {
		if err := runSelfTest(d, ct, got, o.server, o.network, o.name, o.authKey, o.selfTest); err != nil {
			return err
		}
		return nil
	}

	if o.advExit || o.advRoutes != "" || o.useExit != "" {
		var routes []string
		if o.advRoutes != "" {
			routes = strings.Split(o.advRoutes, ",")
		}
		if err := d.SetRouting(o.advExit, routes, o.useExit); err != nil {
			log.Printf("mnetd: routing: %v", err)
		}
	}
	if o.up {
		go func() {
			if err := d.Up(o.server, o.network, o.name, o.authKey); err != nil {
				log.Printf("mnetd: up: %v", err)
			}
		}()
	}

	go func() {
		if err := d.ServeLocalAPI(); err != nil {
			log.Printf("mnetd: local api: %v", err)
		}
	}()
	if o.sessHost {
		allow := map[string]bool{}
		for _, n := range strings.Split(o.sessAllow, ",") {
			if n = strings.TrimSpace(n); n != "" {
				allow[n] = true
			}
		}
		go runSessionHost(d, o.stateDir, o.sessAuto, allow, o.sessRec)
	}
	log.Printf("mnetd %s ready (state: %s)", version.Version, o.stateDir)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	if stop != nil {
		select {
		case <-sig:
		case <-stop:
		}
	} else {
		<-sig
	}
	log.Printf("mnetd: shutting down")
	d.Down()
	return nil
}
