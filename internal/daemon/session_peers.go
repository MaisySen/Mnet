package daemon

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// SessionPeer is private identity evidence for session admission, available
// in-process and through the capability-protected local daemon API only.
// It is not part of Status, public peer-map responses or a display-name login.
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

// MatchSessionPeer resolves one current approved peer by canonical full key,
// unique case-insensitive name and unique TCP source address. The supplied
// snapshot must come from SessionPeers, not display-only Status peers.
func MatchSessionPeer(peers []SessionPeer, claim string, from net.Addr) (string, string, bool) {
	tcp, ok := from.(*net.TCPAddr)
	if !ok || tcp == nil {
		return "", "", false
	}
	source, ok := netip.AddrFromSlice(tcp.IP)
	if !ok {
		return "", "", false
	}
	source = source.Unmap()
	if claim == "" || strings.TrimSpace(claim) != claim {
		return "", "", false
	}
	result := ""
	key := ""
	sourceMatches, nameMatches := 0, 0
	for _, peer := range peers {
		ip, err := netip.ParseAddr(peer.IPv4)
		sameSource := err == nil && ip.Unmap() == source
		sameName := strings.EqualFold(peer.Name, claim)
		if sameSource {
			sourceMatches++
		}
		if sameName {
			nameMatches++
		}
		if sameSource && sameName {
			result = peer.Name
			key = peer.PublicKey
		}
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(key)
	validKey := err == nil && len(raw) == 32 && base64.StdEncoding.EncodeToString(raw) == key
	return result, key, result != "" && sourceMatches == 1 && nameMatches == 1 && validKey
}

// serveSessionPeers is registered only behind the local daemon capability.
// Full peer keys remain outside ordinary display Status and public control APIs.
func (d *Daemon) serveSessionPeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d.SessionPeers())
}
