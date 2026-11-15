package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"fintex.au/mnet/internal/session"
)

// mnet session connect <peer> [--save-frames DIR] [--send-file PATH]
// mnet session send <peer> <file>
//
// The CLI viewer: chat in the terminal, screen frames optionally
// written to disk, file push. Input injection from a terminal is
// chat and files; the interactive viewer lives in the app.
func runSession(args []string) {
	if len(args) < 2 {
		fatal("usage: mnet session connect <peer> [--save-frames DIR] [--send-file PATH]\n" +
			"       mnet session send <peer> <file>")
	}
	switch args[0] {
	case "connect":
		peer := args[1]
		saveDir := ""
		sendFile := ""
		for i := 2; i+1 < len(args); i++ {
			switch args[i] {
			case "--save-frames":
				saveDir = args[i+1]
			case "--send-file":
				sendFile = args[i+1]
			}
		}
		sessionConnect(peer, saveDir, sendFile)
	case "send":
		if len(args) < 3 {
			fatal("usage: mnet session send <peer> <file>")
		}
		sessionConnectSend(args[1], args[2])
	default:
		fatal("unknown session subcommand: " + args[0])
	}
}

func peerIP(peer string) string {
	st := status(client(stateDir))
	for _, p := range st.Peers {
		if p.Name == peer || p.IPv4 == peer || p.FQDN == peer {
			return p.IPv4
		}
	}
	fatal("no such peer: " + peer)
	return ""
}

func dialSession(peer string) (*session.Mux, string) {
	st := status(client(stateDir))
	if st.Status != "up" {
		fatal("not up")
	}
	ip := peerIP(peer)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "5909"), 10*time.Second)
	must(err)
	m, err := session.Dial(conn, st.NodeName)
	must(err)
	return m, st.NodeName
}

func sessionConnect(peer, saveDir, sendPath string) {
	// Chat lines from stdin flow to whatever connection is current.
	lines := make(chan string, 16)
	stdinDone := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		fmt.Print("> ")
		for sc.Scan() {
			if line := sc.Text(); line != "" {
				lines <- line
			}
			fmt.Print("> ")
		}
		close(stdinDone)
	}()

	backoff := 500 * time.Millisecond
	first := true
	sentFile := false
	for {
		if !first {
			fmt.Printf("\nreconnecting in %s...\n", backoff)
			select {
			case <-stdinDone:
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 15*time.Second {
				backoff = 15 * time.Second
			}
		}
		m, name := dialSessionTry(peer)
		if m == nil {
			first = false
			continue
		}
		backoff = 500 * time.Millisecond
		first = false
		fmt.Printf("session to %s established (chat lines go to the host; ctrl-c ends)\n", peer)
		v := session.ConnectViewer(m)
		wireViewer(v, name, saveDir)
		if sendPath != "" && !sentFile {
			sentFile = true
			go func() {
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
			case <-stdinDone:
				m.Close()
				fmt.Println("\nsession ended")
				return
			case line := <-lines:
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

// dialSessionTry dials once, reporting and swallowing errors so the
// reconnect loop can retry. Fatal config problems still exit.
func dialSessionTry(peer string) (*session.Mux, string) {
	st := status(client(stateDir))
	if st.Status != "up" {
		fatal("not up")
	}
	ip := peerIP(peer)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "5909"), 10*time.Second)
	if err != nil {
		fmt.Printf("dial %s: %v\n", peer, err)
		return nil, ""
	}
	m, err := session.Dial(conn, st.NodeName)
	if err != nil {
		conn.Close()
		fmt.Printf("session handshake: %v\n", err)
		return nil, ""
	}
	return m, st.NodeName
}

// wireViewer attaches the frame and chat handlers for one connection.
func wireViewer(v *session.Viewer, name, saveDir string) {
	if saveDir != "" {
		must(os.MkdirAll(saveDir, 0o755))
		var n uint32
		v.SetOnFrame(func(fr session.ScreenFrame) {
			if fr.Seq <= n {
				return
			}
			n = fr.Seq
			p := filepath.Join(saveDir, "latest.jpg")
			if err := os.WriteFile(p, fr.Image, 0o644); err == nil {
				fmt.Printf("\rframe %d (%d bytes) -> %s\n", fr.Seq, len(fr.Image), p)
			}
		})
	}
	v.SetOnChat(func(cm session.ChatMessage) {
		fmt.Printf("\n%s: %s\n> ", cm.From, cm.Text)
	})
}

func sessionConnectSend(peer, path string) {
	m, _ := dialSession(peer)
	defer m.Close()
	if err := session.SendFile(m, path); err != nil {
		fatal("send: " + err.Error())
	}
	fmt.Println("sent:", path)
}
