//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/base64"
	"fintex.au/mnet/internal/daemon"
	"golang.org/x/sys/unix"
	"net/netip"
	"time"
)

// Inherited authority, NOT decoded DATA. The concrete implementation is minted
// by endpointResidentBootstrapSource from fixed inherited pipe provenance,
// verified parent/process/ns lifetime and one exact parent ADMISSION. Native
// ownership stays in endpointParentSource. No public bool/data constructor.
type inheritedEndpointAdmission interface {
	checkResidentSource(context.Context, *namespaceCustody, Binding, uint64) error
	endpointInheritedChannelOwner()
}

// Parent-only comparison. It returns DATA for the future channel writer, never
// resident admission authority. Original child pidfd/ns/start identity retained
// by the launcher must still match before and after the native map comparison.
func compareEndpointPreparedParentSource(ctx context.Context, c *residentChild, peer *daemon.DevelopmentEndpointPeer, bridge *daemon.DevelopmentEndpointBridge, b Binding, p endpointPreparedBody) (endpointAdmissionBody, error) {
	var zero endpointAdmissionBody
	if !kernelExecutionReleased {
		return zero, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || c == nil || c.cmd == nil || c.cmd.Process == nil || c.nsfd == nil || !c.identityObserved || !p.valid() || p.PID != uint32(c.cmd.Process.Pid) || p.StartTicks != c.startTicks || !sameNamespace(p.Namespace, c.ns) || !sameNamespace(p.Namespace, b.Namespace) {
		return zero, ErrConflict
	}
	if err := pidfdAlive(c.pidfd); err != nil {
		return zero, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(c.nsfd.Fd()), &st); err != nil || uint64(st.Dev) != p.Namespace.Device || st.Ino != p.Namespace.Inode {
		return zero, ErrConflict
	}
	ticks, err := readProcessStartTicks(c.cmd.Process.Pid)
	if err != nil || ticks != p.StartTicks {
		return zero, ErrConflict
	}
	if err = requireEndpointPeerBinding(peer, b); err != nil {
		return zero, err
	}
	self, other, err := peer.Snapshot()
	if err != nil {
		return zero, err
	}
	ownKey, err := endpointKey(b.OwnKey)
	if err != nil {
		return zero, err
	}
	peerKey, err := endpointKey(b.PeerKey)
	if err != nil {
		return zero, err
	}
	ownIP, err := netip.ParseAddr(self.IPv4)
	if err != nil || !ownIP.Is4() {
		return zero, ErrConflict
	}
	peerIP, err := netip.ParseAddr(other.IPv4)
	if err != nil || !peerIP.Is4() {
		return zero, ErrConflict
	}
	if p.OwnKey != ownKey || p.PeerKey != peerKey || p.Own != ownIP.As4() || p.Peer != peerIP.As4() || p.SelfUntilMS != b.SelfExpiry.UnixMilli() || p.PeerUntilMS != b.PeerExpiry.UnixMilli() || time.Now().UnixMilli() >= p.SelfUntilMS || time.Now().UnixMilli() >= p.PeerUntilMS {
		return zero, ErrConflict
	}
	out := endpointAdmissionBody{Prepared: p, SelfRevision: uint64(self.Revision), PeerRevision: uint64(other.Revision)}
	a, err := base64.RawURLEncoding.Strict().DecodeString(self.PublicationID)
	if err != nil || len(a) != 32 {
		return zero, ErrConflict
	}
	copy(out.SelfPublication[:], a)
	a, err = base64.RawURLEncoding.Strict().DecodeString(other.PublicationID)
	if err != nil || len(a) != 32 {
		return zero, ErrConflict
	}
	copy(out.PeerPublication[:], a)
	owned, err := bridge.Current(peer)
	if err != nil || owned.Generation < 2 || owned.PairDigest == ([32]byte{}) || owned.OwnKey != self.PublicKey || owned.PeerKey != other.PublicKey || owned.OwnIP != self.IPv4 || owned.PeerIP != other.IPv4 {
		return out, errEndpointFrame
	}
	out.BridgeGeneration = owned.Generation
	out.BridgePairDigest = owned.PairDigest
	if _, err = out.encode(); err != nil {
		return zero, err
	}
	// Detect native replacement during comparison; expiry alone cannot detect
	// withdrawal/republication or original credential recreation.
	againSelf, againPeer, err := peer.Snapshot()
	if err != nil || againSelf != self || againPeer != other || ctx.Err() != nil {
		return zero, ErrConflict
	}
	if err = pidfdAlive(c.pidfd); err != nil {
		return zero, err
	}
	return out, nil
}
