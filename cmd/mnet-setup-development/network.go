//go:build mnet_enrollment_development

package main

// Explicit disposable network-only command flow. The ordinary setup path is
// unchanged. No OS TUN/listener, legacy client, RD host or input is started.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/proofnet"
	"fmt"
	"golang.zx2c4.com/wireguard/tun"
	"html/template"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func runDisposableNetworkTCP(ctx context.Context, root string, open bool, emit func(daemon.DevelopmentEnrollmentEvent) error) error {
	var network *proofnet.Net
	verified, retired := false, false
	retirementReason := ""
	options := daemon.DevelopmentNetworkOptions{Limits: controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 16}, Device: func(ip string) (tun.Device, error) {
		address, err := netip.ParseAddr(ip)
		if err != nil || !address.Is4() {
			return nil, fmt.Errorf("invalid disposable assigned address")
		}
		injected, n, err := proofnet.CreateNetTUN([]netip.Addr{address}, nil, 1280)
		network = n
		return injected, err
	}, Run: func(ctx context.Context, s *daemon.DevelopmentNetworkSession) error {
		peers := s.Peers()
		// This narrow proof requires one explicitly local reachable counterpart.
		// Ambiguity/expired peers fail rather than selecting a lookalike by name.
		var selected *daemon.DevelopmentNetworkPeer
		for i := range peers {
			p := peers[i]
			if !p.HasEndpoints {
				continue
			}
			// Endpoint presence is only a candidate, not a reachability claim.
			// More than one candidate is ambiguous and must not be guessed.
			if selected == nil {
				copy := p
				selected = &copy
			} else {
				return fmt.Errorf("disposable TCP proof requires one reachable peer")
			}
		}
		if selected == nil {
			return fmt.Errorf("no unexpired disposable TCP counterpart; joining remains saved")
		}
		pin := daemon.SessionPeer{Name: selected.Name, IPv4: selected.IPv4, PublicKey: selected.PublicKey}
		scope, err := s.BindPeer(pin)
		if err != nil {
			return err
		}
		defer scope.Close()
		bounded, cancel := context.WithTimeout(scope.Context(), 3*time.Second)
		defer cancel()
		conn, err := network.DialContextTCPAddrPort(bounded, netip.AddrPortFrom(netip.MustParseAddr(pin.IPv4), 49173))
		if err != nil {
			return err
		}
		if err := scope.Own(conn); err != nil {
			return err
		}
		remote, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok || !remote.IP.Equal(net.ParseIP(pin.IPv4)) {
			return fmt.Errorf("disposable TCP remote did not match pinned peer")
		}
		current, err := s.CurrentSessionPeers()
		if err != nil {
			return err
		}
		found := false
		for _, p := range current {
			if p == pin {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("disposable TCP peer authorization changed during dial")
		}
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		var challenge [32]byte
		if _, err := rand.Read(challenge[:]); err != nil {
			return err
		}
		if _, err := conn.Write(challenge[:]); err != nil {
			return err
		}
		var response [32]byte
		if _, err := io.ReadFull(conn, response[:]); err != nil {
			return err
		}
		expected := sha256.Sum256(challenge[:])
		if response != expected {
			return fmt.Errorf("disposable TCP response did not match fresh challenge")
		}
		verified = true
		if err := emit(daemon.DevelopmentEnrollmentEvent{Stage: "development-network-verified"}); err != nil {
			return err
		}
		// Finite explicit proof only, within the original receipt and immutable
		// request budget. A failed check ends the session, never renews/reconnects.
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-scope.Context().Done():
				retired = true
				retirementReason = scope.Reason()
				return nil
			case <-ticker.C:
				if err := s.Refresh(); err != nil {
					return err
				}
			}
		}
	}}
	if err := daemon.RunDisposableEnrollmentNetwork(ctx, root, emit, options); err != nil {
		return err
	}
	if !verified || !retired {
		return fmt.Errorf("disposable network ended without verified peer retirement")
	}
	// This event follows session Close and its joined device/resource owners.
	if err := emit(daemon.DevelopmentEnrollmentEvent{Stage: "development-network-finished"}); err != nil {
		return err
	}
	reason := "A later source check removed that peer."
	if retirementReason == "peer-expired" {
		reason = "That peer's original approved lease ended."
	}
	file, err := os.OpenFile(filepath.Join(root, "network-result.html"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		page := template.Must(template.New("network-result").Parse(`<!doctype html><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><title>Mnet disposable connection result</title><style>body{font:17px system-ui;background:#f4f5f7;color:#202124;margin:0;padding:64px 24px}main{max-width:620px;margin:auto;background:white;border:1px solid #ddd;border-radius:18px;padding:32px}p{line-height:1.6}small{color:#596171}</style><main><small>DISPOSABLE DEVELOPMENT NETWORK</small><h1>Encrypted TCP verified. Session ended.</h1><p>This local development run exchanged fresh TCP data with the exact approved Mnet peer over WireGuard. {{.Reason}} Its connection was closed and this network session stopped.</p><p>This is a historical result, not a live connection indicator. No OS network interface, screen, keyboard or mouse access was started. You can close this tab.</p></main>`))
		err = page.Execute(file, struct{ Reason string }{reason})
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err == nil && open && runtime.GOOS == "linux" {
		err = runDevelopmentBrowser(context.Background(), filepath.Join(root, "network-result.html"), nil)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Verified disposable TCP session ended; optional historical display unavailable.")
	}
	return nil
}
