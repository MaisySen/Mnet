package daemon

import "time"

// SessionPeer is private in-process identity evidence for session admission.
// It is not part of Status, peer-map HTTP responses or a display-name login.
// Callers must bind the source address and recheck the full key after consent.
type SessionPeer struct{ Name, IPv4, PublicKey string }

func (d *Daemon) SessionPeers() []SessionPeer {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != "up" || d.lastMap == nil {
		return nil
	}
	now := time.Now()
	if !d.lastMap.Self.Approved || expiredAt(d.lastMap.Self, now) {
		return nil
	}
	var peers []SessionPeer
	for _, peer := range d.lastMap.Peers {
		if peer.Approved && !expiredAt(peer, now) {
			peers = append(peers, SessionPeer{Name: peer.Name, IPv4: peer.IPv4, PublicKey: peer.PubKey})
		}
	}
	return peers
}
