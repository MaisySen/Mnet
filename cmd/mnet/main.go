// mnet is the Mnet command line. It talks to a running mnetd over the
// local API.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/localapi"
	"fintex.au/mnet/internal/version"
)

func defaultStateDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "mnet")
	}
	return "/var/lib/mnet"
}

func client(stateDir string) *http.Client {
	network, addr := daemon.SocketPath(stateDir)
	var base http.RoundTripper = http.DefaultTransport
	if network == "unix" {
		base = &http.Transport{Dial: func(_, _ string) (net.Conn, error) { return net.Dial("unix", addr) }}
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: localapi.Transport{StateDir: stateDir, Base: base}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func apiBase() string {
	if runtime.GOOS == "windows" {
		return "http://127.0.0.1:47373"
	}
	return "http://mnetd" // dummy host, the unix socket carries it
}

// stateDir is set in main from --state before any subcommand runs.
var stateDir string

func main() {
	stateDir = defaultStateDir()
	args := os.Args[1:]
	for len(args) > 0 && args[0] == "--state" {
		if len(args) < 2 {
			fatal("--state needs a directory")
		}
		stateDir = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		usage()
		return
	}
	hc := client(stateDir)

	switch args[0] {
	case "version":
		fmt.Printf("mnet %s\n", version.Version)
	case "up":
		upFlags := map[string]any{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--server", "--network", "--name", "--auth-key", "--exit-node":
				if i+1 >= len(args) {
					fatal(args[i] + " needs a value")
				}
				key := strings.TrimPrefix(args[i], "--")
				upFlags[strings.ReplaceAll(key, "-", "_")] = args[i+1]
				i++
			case "--advertise-routes":
				if i+1 >= len(args) {
					fatal("--advertise-routes needs a value")
				}
				upFlags["advertise_routes"] = strings.Split(args[i+1], ",")
				i++
			case "--reset-routing":
				upFlags["reset_routing"] = true
			case "--advertise-exit-node":
				upFlags["advertise_exit_node"] = true
			default:
				fatal("unknown flag " + args[i])
			}
		}
		b, _ := json.Marshal(upFlags)
		r, err := hc.Post(apiBase()+"/localapi/up", "application/json", bytes.NewReader(b))
		must(err)
		checkResponse(r)
		r.Body.Close()
		fmt.Println("starting up - watch `mnet status`")
		// Give it a beat, then surface an approval link if there is one.
		time.Sleep(1500 * time.Millisecond)
		st := status(hc)
		if st.Status == "waiting-approval" {
			fmt.Println("\nlog in / approve this device here:")
			fmt.Println(st.ApprovalURL)
		}
	case "wait":
		timeout, err := waitTimeout(args[1:])
		must(err)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		st, err := waitUntilUp(ctx, hc, apiBase(), 250*time.Millisecond)
		if err != nil {
			if st != nil && st.Status == "waiting-approval" && st.ApprovalURL != "" {
				fmt.Println("approve this device using the private link from mnet status")
			}
			fatal(err.Error())
		}
		fmt.Printf("up: %s\n", st.IPv4)
	case "down":
		r, err := hc.Post(apiBase()+"/localapi/down", "application/json", nil)
		must(err)
		checkResponse(r)
		r.Body.Close()
		fmt.Println("down")
	case "status":
		asJSON, err := statusJSONRequested(args[1:])
		must(err)
		st := status(hc)
		if asJSON {
			must(writeJSONStatus(os.Stdout, st))
		} else {
			printStatus(st)
		}
	case "devices":
		if len(args) != 1 {
			fatal("usage: mnet devices (read-only Linux X11 snapshot)")
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		must(runDeviceWindow(ctx, hc, apiBase()))
	case "ip":
		st := status(hc)
		if st.Status != "up" || st.IPv4 == "" {
			fatal("not up")
		}
		fmt.Println(st.IPv4)
	case "desk":
		runDesk(args[1:])
	case "session":
		runSession(args[1:])
	case "ping":
		if len(args) != 2 {
			fatal("usage: mnet ping <exact-peer>")
		}
		target, err := selectPingTarget(args[1], status(hc))
		must(err)
		ping(target)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`mnet - the Mnet command line

  mnet up --server https://control.example.com --network mynet [--auth-key KEY] [--name NAME]
          [--advertise-routes CIDR,...] [--reset-routing]
          Exit routing is unsupported. --reset-routing clears saved routing roles while down.
  mnet down
  mnet wait [--timeout 60s]  (observe current Up state; timeout does not stop startup)
  mnet status [--json]  (private local status; may include an approval link)
  mnet devices  (read-only Linux X11 snapshot, manual refresh; no network/session changes)
  mnet ip
  mnet ping <node>
  mnet desk layout-check FILE [--edge DISPLAY EDGE PIXEL]  (offline geometry only)
  mnet session browse  (Linux X11 chooser, exact current peer; no automatic control)
  mnet session view <peer>  (attended Linux X11, view-only)
  mnet session point <peer> (attended Linux X11, explicit pointer request; no keyboard/dragging)
  mnet session connect <peer> [--save-frames DIR] [--send-file PATH]
  mnet session send <peer> <file>
  mnet version

Global flag: --state DIR (default: user config dir /mnet)
The daemon (mnetd) must be running, as root or Administrator.
`)
}

func status(hc *http.Client) *daemon.Status {
	r, err := hc.Get(apiBase() + "/localapi/status")
	must(err)
	defer r.Body.Close()
	checkResponse(r)
	var st daemon.Status
	must(json.NewDecoder(r.Body).Decode(&st))
	return &st
}

func printStatus(st *daemon.Status) {
	writeHumanStatus(os.Stdout, st)
}

func writeHumanStatus(out io.Writer, st *daemon.Status) {
	fmt.Fprintf(out, "status:  %s\n", st.Status)
	if st.LastError != "" {
		fmt.Fprintf(out, "error:   %s\n", st.LastError)
	}
	if st.Status == "down" {
		return
	}
	fmt.Fprintf(out, "server:  %s\nnetwork: %s\n", st.Server, st.Network)
	fmt.Fprintf(out, "this node: %s (%s)\n", st.NodeName, st.IPv4)
	if st.Interface != "" {
		fmt.Fprintf(out, "interface: %s\n", st.Interface)
	}
	if st.ApprovalURL != "" {
		fmt.Fprintf(out, "\napprove this device here:\n%s\n", st.ApprovalURL)
		return
	}
	if len(st.Peers) == 0 {
		fmt.Fprintln(out, "\nno peers yet")
		return
	}
	fmt.Fprintln(out)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIP\tSTATUS\tPATH\tLAST HANDSHAKE\tRX/TX")
	for _, p := range st.Peers {
		online := "offline"
		if p.HandshakeState == "unavailable" || p.HandshakeState == "clock-skew" {
			online = "unknown"
		}
		if p.Online {
			online = "online"
		}
		path := "unknown"
		if p.Endpoint != "" {
			path = "direct"
		}
		if p.Relay && p.Endpoint != "" {
			path = "relay"
		}
		handshake := "never"
		switch p.HandshakeState {
		case "unavailable":
			handshake = "unavailable"
		case "clock-skew":
			handshake = "clock skew"
		default:
			if p.LastHandshake >= 0 {
				handshake = fmt.Sprintf("%ds ago", p.LastHandshake)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d/%d\n", p.Name, p.IPv4, online, path, handshake, p.Rx, p.Tx)
	}
	w.Flush()
}

func ping(target string) {
	// ICMP checks reachability to the selected current map address. It does not
	// authenticate a remote person/key, prove application health or grant input.
	out, err := runPing(target)
	if err != nil {
		fatal("ping: " + err.Error())
	}
	fmt.Print(out)
}

func must(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "mnet: "+msg)
	os.Exit(1)
}

func checkResponse(r *http.Response) {
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		fatal(fmt.Sprintf("daemon request failed: HTTP %d", r.StatusCode))
	}
}
