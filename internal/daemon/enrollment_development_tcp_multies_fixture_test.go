//go:build mnet_enrollment_development && multies_native_fixture

package daemon

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/proofnet"
	"fmt"
	"golang.zx2c4.com/wireguard/tun"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func init() { developmentTCPTransport = testDevelopmentActualTCPTransport }
func testDevelopmentActualTCPTransport(t *testing.T, d *Daemon, dir string) {
	t.Helper()
	before, err := os.ReadFile(d.stateOwner.path)
	if err != nil {
		t.Fatal(err)
	}
	limits := controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 16}
	m, err := d.nativeCurrentMap(context.Background(), limits)
	if err != nil {
		t.Fatal(err)
	}
	var network *proofnet.Net
	err = d.runDevelopmentNetwork(context.Background(), m, &DevelopmentNetworkOptions{Limits: limits, Device: func(ip string) (tun.Device, error) {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return nil, err
		}
		injected, n, err := proofnet.CreateNetTUN([]netip.Addr{addr}, nil, 1280)
		network = n
		return injected, err
	}, Run: func(ctx context.Context, s *DevelopmentNetworkSession) error {
		var selected *DevelopmentNetworkPeer
		for _, p := range s.Peers() {
			for _, original := range m.Peers {
				if original.ID == p.ID && len(original.Endpoints) > 0 {
					copy := p
					selected = &copy
				}
			}
		}
		if selected == nil {
			return fmt.Errorf("no actual current local TCP counterpart")
		}
		pin := SessionPeer{Name: selected.Name, IPv4: selected.IPv4, PublicKey: selected.PublicKey}
		scope, err := s.BindPeer(pin)
		if err != nil {
			return err
		}
		defer scope.Close()
		deadline, stop := context.WithTimeout(scope.Context(), 3*time.Second)
		defer stop()
		conn, err := network.DialContextTCPAddrPort(deadline, netip.AddrPortFrom(netip.MustParseAddr(pin.IPv4), 49173))
		if err != nil {
			return err
		}
		if err := scope.Own(conn); err != nil {
			return err
		}
		remote, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok || !remote.IP.Equal(net.ParseIP(pin.IPv4)) {
			return fmt.Errorf("actual userspace remote address is not pinned TCP peer")
		}
		peers, err := s.CurrentSessionPeers()
		if err != nil || !developmentExactPeerPresent(peers, pin) {
			return fmt.Errorf("postdial current key/address/name no longer authorized")
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
			return fmt.Errorf("fresh encrypted TCP response mismatch")
		}
		if err := writeFixtureAtomic(dir, "native-transport-ready", []byte("fresh actual userspace TCP exchange over same enrolled WireGuard key")); err != nil {
			return err
		}
		until := time.NewTimer(3 * time.Second)
		defer until.Stop()
		for {
			if _, err := os.Stat(filepath.Join(dir, "transport-peer-revoked")); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-until.C:
				return fmt.Errorf("TCP counterpart revocation timeout")
			case <-time.After(5 * time.Millisecond):
			}
		}
		if err := s.Refresh(); err != nil {
			return err
		}
		select {
		case <-scope.Context().Done():
		default:
			return fmt.Errorf("withdrawn exact peer did not cancel TCP consumer context")
		}
		if _, err := conn.Read(response[:1]); err == nil {
			return fmt.Errorf("withdrawn exact peer TCP read was not closed")
		}
		deniedCtx, cancel := context.WithTimeout(scope.Context(), time.Second)
		defer cancel()
		if next, err := network.DialContextTCPAddrPort(deniedCtx, netip.AddrPortFrom(netip.MustParseAddr(pin.IPv4), 49173)); err == nil {
			next.Close()
			return fmt.Errorf("withdrawn peer allowed new userspace dial")
		}
		if err := writeFixtureAtomic(dir, "native-tcp-revocation-checked", []byte("exact TCP peer scope canceled, existing conn closed and new dial denied")); err != nil {
			return err
		}
		until.Reset(3 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(dir, "transport-post-removal-verified")); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-until.C:
				return fmt.Errorf("TCP host retirement not joined")
			case <-time.After(5 * time.Millisecond):
			}
		}
		return nil
	}})
	if err != nil {
		t.Fatal("actual finite native userspace TCP journey", err)
	}
	after, err := os.ReadFile(d.stateOwner.path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("TCP journey rewrote native State")
	}
	d.deviceMu.RLock()
	running := d.dev != nil
	d.deviceMu.RUnlock()
	if running {
		t.Fatal("TCP journey did not join device on stop")
	}
	t.Log("actual userspace TCP net.Conn pinned address/key/name and fresh response verified; withdrawn scope canceled/closed/denied; native owner stopped")
}
