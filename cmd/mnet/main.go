// mnet is the Mnet command line. It talks to a running mnetd over the
// local API.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"text/tabwriter"
	"time"

	"fintex.au/mnet/internal/daemon"
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
	if network == "tcp" {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Dial: func(_, _ string) (net.Conn, error) {
				return net.Dial("unix", addr)
			},
		},
	}
}

func apiBase() string {
	if runtime.GOOS == "windows" {
		return "http://127.0.0.1:47373"
	}
	return "http://mnetd" // dummy host, the unix socket carries it
}

func main() {
	stateDir := defaultStateDir()
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
		upFlags := map[string]string{}
		for i := 1; i+1 < len(args); i += 2 {
			switch args[i] {
			case "--server":
				upFlags["server"] = args[i+1]
			case "--network":
				upFlags["network"] = args[i+1]
			case "--name":
				upFlags["name"] = args[i+1]
			case "--auth-key":
				upFlags["auth_key"] = args[i+1]
			default:
				fatal("unknown flag " + args[i])
			}
		}
		b, _ := json.Marshal(upFlags)
		r, err := hc.Post(apiBase()+"/localapi/up", "application/json", bytes.NewReader(b))
		must(err)
		r.Body.Close()
		fmt.Println("starting up - watch `mnet status`")
		// Give it a beat, then surface an approval link if there is one.
		time.Sleep(1500 * time.Millisecond)
		st := status(hc)
		if st.Status == "waiting-approval" {
			fmt.Println("\napprove this device here:")
			fmt.Println(st.ApprovalURL)
		}
	case "down":
		r, err := hc.Post(apiBase()+"/localapi/down", "application/json", nil)
		must(err)
		r.Body.Close()
		fmt.Println("down")
	case "status":
		printStatus(status(hc))
	case "ip":
		st := status(hc)
		if st.IPv4 == "" {
			fatal("not up")
		}
		fmt.Println(st.IPv4)
	case "ping":
		if len(args) < 2 {
			fatal("usage: mnet ping <node>")
		}
		st := status(hc)
		target := ""
		for _, p := range st.Peers {
			if p.Name == args[1] || p.IPv4 == args[1] || p.FQDN == args[1] {
				target = p.IPv4
			}
		}
		if target == "" {
			fatal("no such peer: " + args[1])
		}
		ping(target)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`mnet - the Mnet command line

  mnet up --server https://control.example.com --network mynet [--auth-key KEY] [--name NAME]
  mnet down
  mnet status
  mnet ip
  mnet ping <node>
  mnet version

Global flag: --state DIR (default: user config dir /mnet)
The daemon (mnetd) must be running, as root or Administrator.
`)
}

func status(hc *http.Client) *daemon.Status {
	r, err := hc.Get(apiBase() + "/localapi/status")
	must(err)
	defer r.Body.Close()
	var st daemon.Status
	must(json.NewDecoder(r.Body).Decode(&st))
	return &st
}

func printStatus(st *daemon.Status) {
	fmt.Printf("status:  %s\n", st.Status)
	if st.LastError != "" {
		fmt.Printf("error:   %s\n", st.LastError)
	}
	if st.Status == "down" {
		return
	}
	fmt.Printf("server:  %s\nnetwork: %s\n", st.Server, st.Network)
	fmt.Printf("this node: %s (%s)\n", st.NodeName, st.IPv4)
	if st.ApprovalURL != "" {
		fmt.Printf("\napprove this device here:\n%s\n", st.ApprovalURL)
		return
	}
	if len(st.Peers) == 0 {
		fmt.Println("\nno peers yet")
		return
	}
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIP\tSTATUS\tPATH\tLAST HANDSHAKE\tRX/TX")
	for _, p := range st.Peers {
		online := "offline"
		if p.Online {
			online = "online"
		}
		path := "direct"
		if p.Relay {
			path = "relay"
		}
		handshake := "never"
		if p.LastHandshake >= 0 {
			handshake = fmt.Sprintf("%ds ago", p.LastHandshake)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d/%d\n", p.Name, p.IPv4, online, path, handshake, p.Rx, p.Tx)
	}
	w.Flush()
}

func ping(target string) {
	// Plain ICMP through the mesh proves the tunnel end to end.
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
