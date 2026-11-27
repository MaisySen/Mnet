package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/privatefs"
	"fintex.au/mnet/internal/session"
)

// mnet session connect <peer> [--save-frames DIR] [--send-file PATH]
// mnet session send <peer> <file>
// mnet session log                 hosted-session audit on this node
//
// The CLI viewer: chat in the terminal, screen frames optionally
// written to disk, file push. Input injection from a terminal is
// chat and files; the interactive viewer lives in the app.
func runSession(args []string) {
	if len(args) < 1 || (args[0] != "log" && args[0] != "browse" && len(args) < 2) {
		fatal("usage: mnet session browse\n" +
			"       mnet session view <peer>\n" +
			"       mnet session connect <peer> [--save-frames DIR] [--send-file PATH]\n" +
			"       mnet session send <peer> <file>\n" +
			"       mnet session log")
	}
	switch args[0] {
	case "browse":
		if len(args) != 1 {
			fatal("usage: mnet session browse")
			return
		}
		sessionBrowse()
	case "view":
		if len(args) != 2 {
			fatal("usage: mnet session view <peer>")
			return
		}
		sessionView(args[1])
	case "point":
		if len(args) != 2 {
			fatal("usage: mnet session point <peer> (attended pointer only)")
			return
		}
		sessionPoint(args[1])
	case "connect":
		peer := args[1]
		saveDir, sendFile, err := parseSessionConnectOptions(args[2:])
		if err != nil {
			fatal(err.Error())
			return
		}
		sessionConnect(peer, saveDir, sendFile)
	case "send":
		if len(args) < 3 {
			fatal("usage: mnet session send <peer> <file>")
		}
		sessionConnectSend(args[1], args[2])
	case "log":
		sessionLog()
	default:
		fatal("unknown session subcommand: " + args[0])
	}
}

var errCLISessionIdentity = errors.New("session host identity unavailable or changed")

func dialAuthorizedCLI(conn net.Conn, name string, budget time.Duration, peer daemon.SessionPeer, lookup func() ([]daemon.SessionPeer, error)) (*session.Mux, error) {
	return dialAuthorizedCLIMode(conn, name, budget, peer, lookup, false)
}
func dialAuthorizedCLIMode(conn net.Conn, name string, budget time.Duration, peer daemon.SessionPeer, lookup func() ([]daemon.SessionPeer, error), attended bool) (*session.Mux, error) {
	if lookup == nil {
		conn.Close()
		return nil, fmt.Errorf("session authorization unavailable")
	}
	check := func(claim string, source net.Addr) bool {
		peers, err := lookup()
		if err != nil {
			return false
		}
		name, key, ok := daemon.MatchSessionPeer(peers, claim, source)
		return ok && name == peer.Name && key == peer.PublicKey
	}
	source := conn.RemoteAddr()
	remote, ok := source.(*net.TCPAddr)
	if !ok || remote == nil || !remote.IP.Equal(net.ParseIP(peer.IPv4)) || !check(peer.Name, source) {
		conn.Close()
		return nil, errCLISessionIdentity
	}
	if err := conn.SetDeadline(time.Now().Add(budget)); err != nil {
		conn.Close()
		return nil, err
	}
	dial := session.DialWithPeerVerifier
	if attended {
		dial = session.DialAttendedView
	}
	mux, err := dial(conn, name, func(reply session.HelloReply, from net.Addr) error {
		if !check(reply.NodeName, from) {
			return errCLISessionIdentity
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		mux.Close()
		return nil, err
	}
	return mux, nil
}

type cliSessionAuthorization struct {
	peer            daemon.SessionPeer
	lookup          func() ([]daemon.SessionPeer, error)
	denied          atomic.Bool
	requireAttended bool
}

func (a *cliSessionAuthorization) check() bool {
	if a.denied.Load() {
		return false
	}
	peers, err := a.lookup()
	name, key, ok := daemon.MatchSessionPeer(peers, a.peer.Name, &net.TCPAddr{IP: net.ParseIP(a.peer.IPv4)})
	if err != nil || !ok || name != a.peer.Name || key != a.peer.PublicKey {
		a.denied.Store(true)
		return false
	}
	return !a.denied.Load()
}
func (a *cliSessionAuthorization) watch(m *session.Mux) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.Closed():
			return
		case <-ticker.C:
		}
		if !a.check() {
			m.Close()
			return
		}
	}
}
func readCLISessionPeers(hc *http.Client) ([]daemon.SessionPeer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase()+"/localapi/session-peers", nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local session authorization unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local session authorization unavailable (HTTP %d)", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return nil, fmt.Errorf("invalid local session evidence")
	}
	var peers []daemon.SessionPeer
	if json.Unmarshal(b, &peers) != nil {
		return nil, fmt.Errorf("invalid local session evidence")
	}
	return peers, nil
}
func selectCLISessionPeer(selector string, st *daemon.Status, peers []daemon.SessionPeer) (daemon.SessionPeer, error) {
	ip, claim := "", ""
	matches := 0
	for _, p := range st.Peers {
		if p.Name == selector || p.IPv4 == selector || p.FQDN == selector {
			ip, claim = p.IPv4, p.Name
			matches++
		}
	}
	if matches != 1 {
		return daemon.SessionPeer{}, fmt.Errorf("session selector must match exactly one current peer")
	}
	source := &net.TCPAddr{IP: net.ParseIP(ip)}
	name, key, ok := daemon.MatchSessionPeer(peers, claim, source)
	if ok {
		return daemon.SessionPeer{Name: name, IPv4: ip, PublicKey: key}, nil
	}
	return daemon.SessionPeer{}, fmt.Errorf("selected peer is not currently authorized")
}
func prepareCLISession(peer string) (*cliSessionAuthorization, string, error) {
	hc := client(stateDir)
	st := status(hc)
	if st.Status != "up" {
		return nil, "", fmt.Errorf("not up")
	}
	lookup := func() ([]daemon.SessionPeer, error) { return readCLISessionPeers(hc) }
	peers, err := lookup()
	if err != nil {
		return nil, "", err
	}
	pinned, err := selectCLISessionPeer(peer, st, peers)
	if err != nil {
		return nil, "", err
	}
	return &cliSessionAuthorization{peer: pinned, lookup: lookup}, st.NodeName, nil
}
func dialSessionTryAuthorized(a *cliSessionAuthorization, name string) (*session.Mux, error) {
	return dialSessionTryAuthorizedContext(context.Background(), a, name)
}
func dialSessionTryAuthorizedContext(ctx context.Context, a *cliSessionAuthorization, name string) (*session.Mux, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !a.check() {
		return nil, fmt.Errorf("session authorization ended; select the peer again after reviewing current identity")
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(a.peer.IPv4, "5909"))
	if err != nil {
		return nil, err
	}
	m, err := dialAuthorizedCLIContextMode(ctx, conn, name, 70*time.Second, a.peer, a.lookup, a.requireAttended)
	if err != nil {
		if errors.Is(err, errCLISessionIdentity) {
			a.denied.Store(true)
		}
		return nil, err
	}
	if !a.check() {
		m.Close()
		return nil, fmt.Errorf("session authorization ended")
	}
	go a.watch(m)
	return m, nil
}

// The run owns its socket through pending hello and active mux lifetime.
func dialAuthorizedCLIContext(ctx context.Context, conn net.Conn, name string, budget time.Duration, peer daemon.SessionPeer, lookup func() ([]daemon.SessionPeer, error)) (*session.Mux, error) {
	return dialAuthorizedCLIContextMode(ctx, conn, name, budget, peer, lookup, false)
}
func dialAuthorizedCLIContextMode(ctx context.Context, conn net.Conn, name string, budget time.Duration, peer daemon.SessionPeer, lookup func() ([]daemon.SessionPeer, error), attended bool) (*session.Mux, error) {
	if err := ctx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	mux, err := dialAuthorizedCLIMode(conn, name, budget, peer, lookup, attended)
	if err != nil {
		stop()
		conn.Close()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		stop()
		mux.Close()
		return nil, err
	}
	go func() { <-mux.Closed(); stop() }()
	return mux, nil
}

// Cannot interrupt an arbitrary Reader inside Read. Queue sends do observe
// cancellation without closing the user's shared stdin descriptor.
func scanSessionLines(ctx context.Context, input io.Reader, output io.Writer, lines chan<- string) {
	sc := bufio.NewScanner(input)
	fmt.Fprint(output, "> ")
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		fmt.Fprint(output, "> ")
	}
}

func sessionConnect(peer, saveDir, sendPath string) {
	access, name, err := prepareCLISession(peer)
	if err != nil {
		fatal(err.Error())
		return
	}
	// EOF cancels pending dial/hello as well as an established connection.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lines := make(chan string, 16)
	go func() { defer cancel(); scanSessionLines(ctx, os.Stdin, os.Stdout, lines) }()

	backoff := 500 * time.Millisecond
	first := true
	sentFile := false
	for {
		if !first {
			fmt.Printf("\nreconnecting in %s...\n", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 15*time.Second {
				backoff = 15 * time.Second
			}
		}
		m, err := dialSessionTryAuthorizedContext(ctx, access, name)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if access.denied.Load() {
				fmt.Println("session ended: current peer authorization unavailable or changed")
				return
			}
			fmt.Printf("session connect failed: %v\n", err)
			first = false
			continue
		}
		backoff = 500 * time.Millisecond
		first = false
		fmt.Printf("session to %s established (chat lines go to the host; ctrl-c ends)\n", peer)
		v := session.ConnectViewer(m)
		wireViewerAuthorized(v, name, saveDir, access)
		if sendPath != "" && !sentFile {
			sentFile = true
			go func() {
				if !access.check() {
					m.Close()
					return
				}
				if err := session.SendFile(m, sendPath); err != nil {
					fmt.Printf("\nfile send failed: %v\n> ", err)
				} else {
					fmt.Printf("\nfile sent: %s\n> ", sendPath)
				}
			}()
		}
		// Pump chat until the session drops or stdin closes. A drop
		// loops back to a redial; the host sends a fresh keyframe, so
		// the picture resumes on its own.
		dead := false
		for !dead {
			select {
			case <-ctx.Done():
				m.Close()
				fmt.Println("\nsession ended")
				return
			case line := <-lines:
				if ctx.Err() != nil {
					m.Close()
					return
				}
				if !access.check() {
					m.Close()
					return
				}
				if err := v.SendChat(name, line); err != nil {
					fmt.Printf("\nchat failed: %v\n", err)
				}
			case <-m.Closed():
				fmt.Println("\nconnection lost")
				dead = true
			}
		}
	}
}

// sessionLog prints this node's hosted-session audit (written by
// mnetd -session-host) newest last.
func sessionLog() {
	b, err := os.ReadFile(filepath.Join(stateDir, "sessions.jsonl"))
	if err != nil {
		fatal("no session log at " + filepath.Join(stateDir, "sessions.jsonl"))
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var ev struct {
			At       time.Time `json:"at"`
			Event    string    `json:"event"`
			Viewer   string    `json:"viewer"`
			SourceIP string    `json:"source_ip"`
			Reason   string    `json:"reason,omitempty"`
			Duration string    `json:"duration,omitempty"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		when := ev.At.Local().Format("2006-01-02 15:04:05")
		out := fmt.Sprintf("%s  %-7s %-20s %-12s", when, ev.Event, ev.Viewer, ev.SourceIP)
		if ev.Duration != "" {
			out += "  " + ev.Duration
		}
		if ev.Reason != "" {
			out += "  " + ev.Reason
		}
		fmt.Println(out)
	}
}

// wireViewer attaches the frame and chat handlers for one connection.
func wireViewerAuthorized(v *session.Viewer, name, saveDir string, access *cliSessionAuthorization) {
	if saveDir != "" {
		must(os.MkdirAll(saveDir, 0o700))
		var n uint32
		v.SetOnFrame(func(fr session.ScreenFrame) {
			if access.denied.Load() {
				return
			}
			if fr.Seq <= n {
				return
			}
			n = fr.Seq
			p := filepath.Join(saveDir, "latest.jpg")
			if err := saveSessionFrame(p, fr.Image); err == nil {
				fmt.Printf("\rframe %d (%d bytes) -> %s\n", fr.Seq, len(fr.Image), p)
			}
		})
	}
	v.SetOnChat(func(cm session.ChatMessage) {
		if access.denied.Load() {
			return
		}
		fmt.Printf("\n%s: %s\n> ", cm.From, cm.Text)
	})
}

func sessionConnectSend(peer, path string) {
	access, name, err := prepareCLISession(peer)
	if err != nil {
		fatal(err.Error())
		return
	}
	m, err := dialSessionTryAuthorized(access, name)
	if err != nil {
		fatal(err.Error())
		return
	}
	defer m.Close()
	if !access.check() {
		fatal("session authorization ended")
		return
	}
	if err := session.SendFile(m, path); err != nil {
		fatal("send: " + err.Error())
	}
	fmt.Println("sent:", path)
}

// saveSessionFrame replaces the latest snapshot via a private same-directory
// temporary file, never by opening/truncating the destination or its symlink.
func saveSessionFrame(path string, data []byte) error {
	f, err := privatefs.CreateTemp(filepath.Dir(path), ".mnet-frame-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	n, err := f.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func parseSessionConnectOptions(args []string) (saveDir, sendFile string, err error) {
	seen := map[string]bool{}
	for i := 0; i < len(args); i += 2 {
		option := args[i]
		if option != "--save-frames" && option != "--send-file" {
			return "", "", fmt.Errorf("unknown session connect option: %s", option)
		}
		if seen[option] {
			return "", "", fmt.Errorf("duplicate session connect option: %s", option)
		}
		if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
			return "", "", fmt.Errorf("%s requires a path (prefix flag-like paths with ./)", option)
		}
		seen[option] = true
		if option == "--save-frames" {
			saveDir = args[i+1]
		} else {
			sendFile = args[i+1]
		}
	}
	return
}
