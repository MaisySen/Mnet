//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"errors"
	"fintex.au/mnet/internal/daemon"
	"time"
)

var errEndpointPeerProfile = errors.New("current-authenticated-endpoint-peer-profile-required")

// Only the actual native session can mint this opaque source owner. Public
// Binding/profile snapshots remain DATA, not a substitute for that owner.
// The remote claim is compatibility under the existing trusted control plane,
// not hardware attestation. Source wrapper is not wired into the subprocess
// launcher: crossing that boundary needs a reviewed authenticated lifetime IPC.
func requireEndpointPeerBinding(peer *daemon.DevelopmentEndpointPeer, b Binding) error {
	if peer == nil || peer.Context() == nil || peer.Context().Err() != nil {
		return errEndpointPeerProfile
	}
	self, other, err := peer.Snapshot()
	if err != nil || self.NetworkID != other.NetworkID || self.PublicKey != b.OwnKey || self.IPv4 != b.OwnIP || other.PublicKey != b.PeerKey || other.IPv4 != b.PeerIP || !time.UnixMilli(self.AccessUntilMS).Equal(b.SelfExpiry) || !time.UnixMilli(other.AccessUntilMS).Equal(b.PeerExpiry) || self.Capability.MTU != MTU || other.Capability.MTU != MTU {
		return errEndpointPeerProfile
	}
	return nil
}

// Local preparation requires actual current native membership, never a peer
// compatibility declaration that cannot exist until local preparation finishes.
func requireEndpointMembershipBinding(peer *daemon.DevelopmentEndpointPeer, b Binding) error {
	if peer == nil || peer.Context() == nil || peer.Context().Err() != nil {
		return errEndpointPeerProfile
	}
	self, other, err := peer.MembershipSnapshot()
	if err != nil || self.KeyExpiry == nil || other.KeyExpiry == nil || self.NetworkID != other.NetworkID || self.PubKey != b.OwnKey || self.IPv4 != b.OwnIP || other.PubKey != b.PeerKey || other.IPv4 != b.PeerIP || !self.KeyExpiry.Equal(b.SelfExpiry) || !other.KeyExpiry.Equal(b.PeerExpiry) {
		return errEndpointPeerProfile
	}
	return nil
}
