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
	m, name := dialSession(peer)
	defer m.Close()
	fmt.Printf("session to %s established (chat lines go to the host; ctrl-c ends)\n", peer)
	v := session.ConnectViewer(m)
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
	if sendPath != "" {
		go func() {
			if err := session.SendFile(m, sendPath); err != nil {
				fmt.Printf("\nfile send failed: %v\n> ", err)
			} else {
				fmt.Printf("\nfile sent: %s\n> ", sendPath)
			}
		}()
	}
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		fmt.Print("> ")
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				continue
			}
			if err := v.SendChat(name, line); err != nil {
				fmt.Printf("\nchat failed: %v\n", err)
				return
			}
			fmt.Print("> ")
		}
		m.Close()
	}()
	<-m.Closed()
	fmt.Println("\nsession ended")
}

func sessionConnectSend(peer, path string) {
	m, _ := dialSession(peer)
	defer m.Close()
	if err := session.SendFile(m, path); err != nil {
		fatal("send: " + err.Error())
	}
	fmt.Println("sent:", path)
}
