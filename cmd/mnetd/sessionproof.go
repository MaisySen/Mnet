package main

// Session proof rig: two in-process nodes on userspace netstack
// tunnels join a network on a real control plane, then run a full
// remote-desktop session between them over the encrypted mesh. It
// exercises capture, dirty-rect frame delivery, chat, file transfer
// and input injection end to end, with no root and no kernel TUN.
//
//	mnetd -session-proof -server https://... -network N -auth-key K -proof-dir DIR
//
// The host side captures the X display named by DISPLAY (point it at
// an Xvfb for a headless rig). Artifacts land in the proof dir:
// frame-first.jpg, frame-latest.jpg, host-files/, summary.json.

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

const proofPort = 5909

type proofNodeT struct {
	d   *daemon.Daemon
	net *netstack.Net
	ip  netip.Addr
}

// proofNode joins the network, learns the assigned address, then comes
// up on a userspace netstack tunnel bound to exactly that address.
func newProofNode(dir, sub, server, network, name, key string, verbose bool) (*proofNodeT, error) {
	return newProofNodeWithTUN(dir, sub, server, network, name, key, verbose, nil)
}

func newProofNodeWithTUN(dir, sub, server, network, name, key string, verbose bool, wrap func(tun.Device) tun.Device) (*proofNodeT, error) {
	d, err := daemon.New(daemon.Config{
		StateDir:     filepath.Join(dir, sub+"-state"),
		Verbose:      verbose,
		ManagedIface: false,
	})
	if err != nil {
		return nil, err
	}
	ipStr, err := d.Prepare(server, network, name, key)
	if err != nil {
		return nil, fmt.Errorf("%s prepare: %w", name, err)
	}
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return nil, fmt.Errorf("%s: bad assigned address %q: %w", name, ipStr, err)
	}
	dev, tnet, err := netstack.CreateNetTUN([]netip.Addr{ip}, nil, 1420)
	if err != nil {
		return nil, fmt.Errorf("%s netstack: %w", name, err)
	}
	if wrap != nil {
		dev = wrap(dev)
	}
	if err := d.SetTUN(dev); err != nil {
		return nil, err
	}
	if err := d.Up(server, network, name, key); err != nil {
		return nil, fmt.Errorf("%s up: %w", name, err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for d.Status().Status != "up" {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%s did not come up", name)
		}
		time.Sleep(250 * time.Millisecond)
	}
	log.Printf("session-proof: %s up at %s", name, ip)
	return &proofNodeT{d: d, net: tnet, ip: ip}, nil
}

type proofResult struct {
	HostIP        string `json:"host_ip"`
	ViewerIP      string `json:"viewer_ip"`
	Frames        int    `json:"frames"`
	FrameWidth    int    `json:"frame_width"`
	FrameHeight   int    `json:"frame_height"`
	FirstFrameB   int    `json:"first_frame_bytes"`
	LatestFrameB  int    `json:"latest_frame_bytes"`
	ChatOK        bool   `json:"chat_ok"`
	FileOK        bool   `json:"file_ok"`
	FileBytes     int64  `json:"file_bytes"`
	FileAckOK     bool   `json:"file_ack_ok"`
	FileSendError string `json:"file_send_error,omitempty"`
	DeltasSmaller bool   `json:"deltas_smaller"`
	Pass          bool   `json:"pass"`
	Fail          string `json:"fail,omitempty"`
}

func runSessionProof(server, network, authKey, dir string, verbose bool) error {
	if server == "" || network == "" {
		return fmt.Errorf("-session-proof needs -server and -network")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	res := &proofResult{}
	fail := func(format string, a ...any) error {
		res.Fail = fmt.Sprintf(format, a...)
		writeProofSummary(dir, res)
		return fmt.Errorf("%s", res.Fail)
	}
	rnd := make([]byte, 4)
	cryptorand.Read(rnd)
	nonce := hex.EncodeToString(rnd)

	host, err := newProofNode(dir, "host", server, network, "proof-host", authKey, verbose)
	if err != nil {
		return fail("host node: %v", err)
	}
	defer host.d.Down()
	res.HostIP = host.ip.String()

	view, err := newProofNode(dir, "view", server, network, "proof-viewer", authKey, verbose)
	if err != nil {
		return fail("viewer node: %v", err)
	}
	defer view.d.Down()
	res.ViewerIP = view.ip.String()

	// Host side: one session on the mesh address, real capture and
	// injection for the platform.
	ln, err := host.net.ListenTCPAddrPort(netip.AddrPortFrom(host.ip, proofPort))
	if err != nil {
		return fail("host listen: %v", err)
	}
	chatGot := make(chan session.ChatMessage, 4)
	fileGot := make(chan string, 4)
	hostErr := make(chan error, 1)
	go func() {
		m, err := session.Listen(ln, "proof-host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		if err != nil {
			hostErr <- fmt.Errorf("session listen: %w", err)
			return
		}
		cap, err := newCapturer("")
		if err != nil {
			hostErr <- fmt.Errorf("capture: %w", err)
			m.Close()
			return
		}
		inj, injErr := newInjector()
		if injErr != nil {
			log.Printf("session-proof: injection unavailable: %v", injErr)
		}

		h := session.ServeHostWithOptions(m, cap, inj, 8, session.HostOptions{
			FileDir: filepath.Join(dir, "host-files"),
			OnChat:  func(cm session.ChatMessage) { chatGot <- cm },
			OnFile:  func(name string, size int64) { fileGot <- name },
		})
		log.Printf("session-proof: session started")
		<-m.Closed()
		h.Stop()
	}()

	// Viewer side: dial through the mesh, retry while the two nodes
	// find a path to each other.
	var m *session.Mux
	deadline := time.Now().Add(90 * time.Second)
	for {
		conn, err := view.net.DialTCPAddrPort(netip.AddrPortFrom(host.ip, proofPort))
		if err == nil {
			m, err = session.Dial(conn, "proof-viewer")
			if err == nil {
				break
			}
			conn.Close()
		}
		if time.Now().After(deadline) {
			return fail("viewer never reached the host through the mesh: %v", err)
		}
		select {
		case herr := <-hostErr:
			return fail("%v", herr)
		case <-time.After(3 * time.Second):
		}
	}
	defer m.Close()
	log.Printf("session-proof: viewer connected to host through the mesh")

	var frameMu sync.Mutex
	var first, latest []byte
	frameCount := 0
	fw, fh := 0, 0
	v := session.ConnectViewerWithOptions(m, session.ViewerOptions{OnFrame: func(fr session.ScreenFrame) {
		frameMu.Lock()
		defer frameMu.Unlock()
		img, err := jpeg.Decode(bytes.NewReader(fr.Image))
		if err != nil {
			log.Printf("session-proof: undecodable frame %d: %v", fr.Seq, err)
			return
		}
		b := img.Bounds()
		fw, fh = b.Dx(), b.Dy()
		frameCount++
		if first == nil {
			first = fr.Image
			os.WriteFile(filepath.Join(dir, "frame-first.jpg"), fr.Image, 0o644)
		}
		latest = fr.Image
		os.WriteFile(filepath.Join(dir, "frame-latest.jpg"), fr.Image, 0o644)
	}})

	// File push: 256 KiB of random bytes, verified by hash.
	payload := make([]byte, 256*1024)
	cryptorand.Read(payload)
	payloadPath := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(payloadPath, payload, 0o644); err != nil {
		return fail("write payload: %v", err)
	}
	wantSum := sha256.Sum256(payload)

	chatText := "session-proof " + nonce
	if err := v.SendChat("proof-viewer", chatText); err != nil {
		return fail("send chat: %v", err)
	}
	fileSent := make(chan error, 1)
	go func() { fileSent <- session.SendFile(m, payloadPath) }()
	// One injected text event; an external watcher (xev on the same
	// display) can prove it reached the X server.
	v.SendInput(session.InputEvent{Kind: "key", Text: "hi"})

	// Collect until everything lands or the clock runs out. A static
	// screen produces a keyframe then silence by design (dirty-rect),
	// so the frame bar is the keyframe plus one changed-content frame
	// after the rig's wallpaper swap, not a frame rate.
	end := time.Now().Add(90 * time.Second)
	chatOK, fileOK := false, false
	for time.Now().Before(end) {
		frameMu.Lock()
		haveFrames := frameCount >= 2
		frameMu.Unlock()
		if haveFrames && chatOK && fileOK && res.FileAckOK {
			break
		}
		select {
		case sendErr := <-fileSent:
			if sendErr != nil {
				res.FileSendError = sendErr.Error()
				return fail("file sender failed: %v", sendErr)
			}
			res.FileAckOK = true
			log.Printf("session-proof: file sender received verified ACK")
		case cm := <-chatGot:
			if cm.Text == chatText {
				chatOK = true
				res.ChatOK = true
			}
		case name := <-fileGot:
			if name == "payload.bin" {
				got, err := os.ReadFile(filepath.Join(dir, "host-files", "payload.bin"))
				if err == nil && sha256.Sum256(got) == wantSum {
					fileOK = true
					res.FileOK = true
					res.FileBytes = int64(len(got))
				}
			}
		case herr := <-hostErr:
			return fail("%v", herr)
		case <-time.After(500 * time.Millisecond):
		}
	}
	frameMu.Lock()
	defer frameMu.Unlock()
	res.Frames = frameCount
	res.FrameWidth, res.FrameHeight = fw, fh
	res.FirstFrameB = len(first)
	res.LatestFrameB = len(latest)
	if frameCount >= 3 && len(latest) > 0 && len(latest) < len(first)*3/4 {
		res.DeltasSmaller = true // static screen: delta frames carry less than the keyframe
	}

	switch {
	case frameCount < 2:
		return fail("initial and changed-content frames did not both arrive in 90s")
	case fw == 0 || fh == 0:
		return fail("frames did not decode")
	case frameCount >= 2 && bytes.Equal(first, latest):
		return fail("the wallpaper swap never arrived as a delta frame")
	case !chatOK:
		return fail("chat line never reached the host")
	case !fileOK:
		return fail("file transfer missing or hash mismatch")
	case !res.FileAckOK:
		return fail("file was received but sender never received verified ACK")
	}
	res.Pass = true
	writeProofSummary(dir, res)
	log.Printf("session-proof: PASS - %d frames %dx%d, chat and %d byte file verified through the mesh",
		frameCount, fw, fh, res.FileBytes)
	return nil
}

func writeProofSummary(dir string, res *proofResult) {
	b, _ := json.MarshalIndent(res, "", "  ")
	os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644)
}
