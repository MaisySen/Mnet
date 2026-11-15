package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/stun"
	"golang.zx2c4.com/wireguard/device"
)

// PeerStatus is the CLI view of one peer.
type PeerStatus struct {
	Name          string `json:"name"`
	IPv4          string `json:"ipv4"`
	FQDN          string `json:"fqdn"`
	PubKeyShort   string `json:"pubkey_short"`
	Endpoint      string `json:"endpoint"`
	Relay         bool   `json:"relay"`
	LastHandshake int64  `json:"last_handshake_ago_sec"`
	Rx            int64  `json:"rx_bytes"`
	Tx            int64  `json:"tx_bytes"`
	Online        bool   `json:"online"`
}

// peerTrack is the path-selection state for one peer.
type peerTrack struct {
	node          netmap.Node
	cycle         int // position in the endpoint rotation
	onRelay       bool
	lastSwitch    time.Time
	lastDirectTry time.Time
	probe         bool // one authenticated direct handshake after endpoint application
}

// applyMap writes the map into the WireGuard device.
func (d *Daemon) applyMap(m *netmap.Map) {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	d.applyMapLocked(m)
	select {
	case d.mapChanged <- struct{}{}:
	default:
	}
}

// refreshPaths uses the latest authorized map after taking the apply lock,
// so a path tick cannot reinstall an older map after peer revocation.
func (d *Daemon) refreshPaths() {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	if m := d.currentMap(); m != nil {
		d.applyMapLocked(m)
	}
}

func (d *Daemon) applyMapLocked(m *netmap.Map) {
	m = unexpiredMap(m, time.Now())
	d.deviceMu.RLock()
	defer d.deviceMu.RUnlock()
	if d.dev == nil {
		return
	}
	var conf bytes.Buffer
	wanted := make(map[string]bool, len(m.Peers))
	for _, p := range m.Peers {
		wanted[p.PubKey] = true
	}
	var removed []string
	d.peersMu.Lock()
	for key := range d.peers {
		if !wanted[key] {
			if raw, err := base64.StdEncoding.DecodeString(key); err == nil && len(raw) == 32 {
				fmt.Fprintf(&conf, "public_key=%s\nremove=true\n", hex.EncodeToString(raw))
			}
			removed = append(removed, key)
		}
	}
	d.peersMu.Unlock()
	var probes []device.NoisePublicKey
	for _, p := range m.Peers {
		p.Endpoints = safePeerEndpoints(p.Endpoints, m.Peers)
		pub, err := base64.StdEncoding.DecodeString(p.PubKey)
		if err != nil || len(pub) != 32 {
			continue
		}
		d.peersMu.Lock()
		st := d.peers[p.PubKey]
		if st == nil {
			st = &peerTrack{node: p}
			d.peers[p.PubKey] = st
		} else {
			st.node = p
		}
		ep := st.currentEndpoint()
		if st.probe && !st.onRelay && !strings.HasPrefix(ep, bind.RelayPrefix) {
			var key device.NoisePublicKey
			copy(key[:], pub)
			probes = append(probes, key)
			st.probe = false
		}
		d.peersMu.Unlock()
		fmt.Fprintf(&conf, "public_key=%s\n", hex.EncodeToString(pub))
		fmt.Fprintf(&conf, "replace_allowed_ips=true\n")
		fmt.Fprintf(&conf, "allowed_ip=%s/32\n", p.IPv4)
		for _, r := range p.Routes {
			fmt.Fprintf(&conf, "allowed_ip=%s\n", r)
		}
		if d.state.UseExitNode != "" && p.ExitNode && (p.Name == d.state.UseExitNode || p.IPv4 == d.state.UseExitNode) {
			fmt.Fprintf(&conf, "allowed_ip=0.0.0.0/0\n")
			fmt.Fprintf(&conf, "allowed_ip=::/0\n")
		}
		fmt.Fprintf(&conf, "persistent_keepalive_interval=25\n")
		if ep != "" {
			fmt.Fprintf(&conf, "endpoint=%s\n", ep)
		}
	}
	if err := d.dev.IpcSetOperation(&conf); err != nil {
		d.setErr(fmt.Errorf("apply map: %w", err))
		return
	}
	for _, key := range probes {
		if peer := d.dev.LookupPeer(key); peer != nil {
			_ = peer.SendHandshakeInitiation(false)
		}
	}
	d.peersMu.Lock()
	for _, key := range removed {
		delete(d.peers, key)
	}
	d.peersMu.Unlock()
	d.mu.Lock()
	d.lastMap = m
	if d.status == "up" || d.status == "key-expired" || d.status == "waiting-approval" {
		if m.Self.KeyExpired() {
			d.status = "key-expired"
		} else if m.Self.Approved {
			d.status = "up"
			d.approvalURL = ""
		}
	}
	d.mu.Unlock()
	if d.subnets != nil {
		var routes []string
		for _, p := range m.Peers {
			routes = append(routes, p.Routes...)
		}
		if err := d.subnets.Sync(routes); err != nil {
			d.setErr(err)
		}
	}
	if d.dns != nil {
		d.dns.Update(m)
	}
}

// Never choose an underlay endpoint that the current mesh routes back
// into the TUN. This can include another peer's advertised subnet.
func safePeerEndpoints(endpoints []string, peers []netmap.Node) []string {
	blocked := []netip.Prefix{netip.MustParsePrefix("100.64.0.0/10")}
	for _, peer := range peers {
		for _, raw := range peer.Routes {
			if prefix, err := netip.ParsePrefix(raw); err == nil {
				blocked = append(blocked, prefix.Masked())
			}
		}
	}
	var safe []string
	for _, endpoint := range endpoints {
		addr, err := netip.ParseAddrPort(endpoint)
		if err != nil {
			continue
		}
		recursive := false
		for _, prefix := range blocked {
			if prefix.Contains(addr.Addr().Unmap()) {
				recursive = true
				break
			}
		}
		if !recursive {
			safe = append(safe, endpoint)
		}
	}
	return safe
}

func (st *peerTrack) currentEndpoint() string {
	if st.onRelay || len(st.node.Endpoints) == 0 {
		return bind.RelayPrefix + st.node.PubKey
	}
	return st.node.Endpoints[st.cycle%len(st.node.Endpoints)]
}

// mapLoop long polls the control plane for new maps.
func (d *Daemon) mapLoop(ctx context.Context) {
	for {
		var since int64
		if m := d.currentMap(); m != nil {
			since = m.Version
		}
		m, err := d.client().WaitMap(ctx, since)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, controlclient.ErrUnauthorized) {
				// Revocation must stop existing dataplane authorization before
				// waiting for a fresh join/approval.
				d.applyMap(&netmap.Map{})
				d.rejoin(ctx)
				continue
			}
			waitContext(ctx, 2*time.Second)
			continue
		}
		if m.Self.Approved {
			d.applyMap(m)
		}
	}
}

// endpointLoop discovers and reports our public and local endpoints.
func (d *Daemon) endpointLoop(ctx context.Context) {
	changes := watchNetworkChanges(ctx)
	for {
		d.discoverEndpoints(ctx)
		select {
		case <-ctx.Done():
			return
		case <-changes:
			// The machine changed networks: re-resolve now so peers
			// get the new endpoint within seconds, not on the next
			// 45 second tick.
			continue
		case <-time.After(45 * time.Second):
		}
	}
}

func (d *Daemon) discoverEndpoints(ctx context.Context) {
	if d.bind == nil || d.client() == nil {
		return
	}
	port := int(d.bind.Port())
	eps := map[string]bool{}

	// Public endpoint via STUN on the WireGuard socket.
	d.stunMu.Lock()
	d.stunResult = ""
	d.stunMu.Unlock()
	req, txid := stun.NewRequest()
	d.stunMu.Lock()
	d.stunTx[txid] = time.Now()
	d.stunMu.Unlock()
	for _, srv := range stun.DefaultServers {
		_ = d.bind.WriteStun(req, srv)
	}
	// Give responses a moment, the handler fills stunResult.
	waitContext(ctx, 1200*time.Millisecond)
	if ctx.Err() != nil {
		return
	}
	d.stunMu.Lock()
	if d.stunResult != "" {
		eps[d.stunResult] = true
	}
	d.stunMu.Unlock()

	// Local LAN addresses help peers on the same network.
	meshIP := ""
	if m := d.currentMap(); m != nil {
		meshIP = m.Self.IPv4
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil {
				continue
			}
			if ip4 := ip.To4(); usableUnderlayIP(ip4, meshIP) {
				eps[fmt.Sprintf("%s:%d", ip4, port)] = true
			}
		}
	}

	out := make([]string, 0, len(eps))
	for e := range eps {
		out = append(out, e)
	}
	if err := d.client().ReportEndpoints(ctx, out); err != nil {
		d.setErr(fmt.Errorf("report endpoints: %w", err))
	}
}

// onStun handles intercepted STUN responses from the bind.
func (d *Daemon) onStun(msg []byte) {
	d.stunMu.Lock()
	defer d.stunMu.Unlock()
	for txid, at := range d.stunTx {
		if time.Since(at) > 10*time.Second {
			delete(d.stunTx, txid)
			continue
		}
		if ep, err := stun.ParseResponse(msg, txid); err == nil {
			d.stunResult = ep
			delete(d.stunTx, txid)
			return
		}
	}
}

// peerStats is parsed device IPC output for one peer.
type peerStats struct {
	endpoint  string
	handshake int64 // unix time
	rx, tx    int64
	keepalive int64
}

// stats reads every peer's counters from the device.
func (d *Daemon) stats() map[string]*peerStats {
	d.deviceMu.RLock()
	defer d.deviceMu.RUnlock()
	out := map[string]*peerStats{}
	if d.dev == nil {
		return out
	}
	var buf bytes.Buffer
	if err := d.dev.IpcGetOperation(&buf); err != nil {
		return out
	}
	return parsePeerStats(buf.String())
}

func parsePeerStats(raw string) map[string]*peerStats {
	out := map[string]*peerStats{}
	var cur *peerStats
	var curKey string
	for _, line := range strings.Split(raw, "\n") {
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch k {
		case "public_key":
			raw, err := hex.DecodeString(v)
			if err != nil || len(raw) != 32 {
				cur = nil
				continue
			}
			curKey = base64.StdEncoding.EncodeToString(raw)
			cur = &peerStats{}
			out[curKey] = cur
		case "endpoint":
			if cur != nil {
				cur.endpoint = v
			}
		case "last_handshake_time_sec":
			if cur != nil {
				cur.handshake, _ = strconv.ParseInt(v, 10, 64)
			}
		case "rx_bytes":
			if cur != nil {
				cur.rx, _ = strconv.ParseInt(v, 10, 64)
			}
		case "tx_bytes":
			if cur != nil {
				cur.tx, _ = strconv.ParseInt(v, 10, 64)
			}
		case "persistent_keepalive_interval":
			if cur != nil {
				cur.keepalive, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	}
	return out
}

// pathLoop moves peers between direct endpoints and the relay.
func (d *Daemon) pathLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if d.dev == nil {
			continue
		}
		changed := d.selectPaths(d.stats(), time.Now())
		if changed {
			d.refreshPaths()
		}
	}
}

// relayLoop keeps the relay socket connected.
func (d *Daemon) relayLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if d.dev == nil || d.client() == nil {
			waitContext(ctx, time.Second)
			continue
		}
		r, err := d.client().DialRelay(ctx, d.state.PublicKey, d.bind.Inject)
		if err != nil {
			if errors.Is(err, controlclient.ErrUnauthorized) {
				// Revocation must stop existing dataplane authorization before
				// waiting for a fresh join/approval.
				d.applyMap(&netmap.Map{})
				d.rejoin(ctx)
				continue
			}
			log.Printf("mnetd: relay dial failed: %v (retrying)", err)
			waitContext(ctx, 3*time.Second)
			continue
		}
		d.mu.Lock()
		d.relay = r
		d.mu.Unlock()
		d.bind.SetRelaySender(r.Send)
		log.Printf("mnetd: relay connected")
		select {
		case <-ctx.Done():
			r.Close()
			return
		case <-r.Done():
		}
		d.bind.SetRelaySender(nil)
		waitContext(ctx, 2*time.Second)
	}
}

// PeerStatuses renders the current peer table for the CLI.
func (d *Daemon) PeerStatuses() []PeerStatus {
	stats := d.stats()
	now := time.Now()
	var out []PeerStatus
	m := d.currentMap()
	if m == nil {
		return out
	}
	for _, p := range m.Peers {
		ps := PeerStatus{
			Name:        p.Name,
			IPv4:        p.IPv4,
			FQDN:        p.FQDN(m.Network.Name),
			PubKeyShort: p.PubKey[:12] + "...",
		}
		if s := stats[p.PubKey]; s != nil {
			ps.Endpoint = s.endpoint
			ps.Relay = strings.HasPrefix(s.endpoint, bind.RelayPrefix)
			if s.handshake > 0 {
				ps.LastHandshake = now.Unix() - s.handshake
			} else {
				ps.LastHandshake = -1
			}
			ps.Rx, ps.Tx = s.rx, s.tx
			ps.Online = s.handshake > 0 && now.Unix()-s.handshake < 180
		}
		out = append(out, ps)
	}
	return out
}

// selectPaths opens a bounded 30-second direct-probe window, then backs
// off for 90 seconds. Authenticated relay traffic can roam the WG endpoint
// back while a probe is pending, so a healthy relay is not direct success.
func (d *Daemon) selectPaths(stats map[string]*peerStats, now time.Time) bool {
	changed := false
	d.peersMu.Lock()
	defer d.peersMu.Unlock()
	for pk, st := range d.peers {
		s := stats[pk]
		healthy := s != nil && s.handshake > 0 && now.Unix()-s.handshake < 90
		direct := s != nil && s.endpoint != "" && !strings.HasPrefix(s.endpoint, bind.RelayPrefix)
		if healthy && direct {
			if st.onRelay {
				st.onRelay = false
				changed = true
			}
			continue
		}
		if len(st.node.Endpoints) == 0 {
			if !st.onRelay {
				st.onRelay = true
				st.lastSwitch = now
				changed = true
			}
			continue
		}
		elapsed := now.Sub(st.lastDirectTry)
		if st.lastDirectTry.IsZero() || elapsed > 90*time.Second {
			st.onRelay = false
			st.probe = true
			st.lastDirectTry = now
			st.lastSwitch = now
			changed = true
			continue
		}
		if elapsed <= 30*time.Second {
			if !st.onRelay && now.Sub(st.lastSwitch) >= 5*time.Second {
				st.cycle++
				st.probe = true
				st.lastSwitch = now
				changed = true
			}
		} else if !st.onRelay {
			st.onRelay = true
			st.lastSwitch = now
			changed = true
		}
	}
	return changed
}

// Never advertise our own overlay address as an underlay endpoint. The
// server sorts endpoints, so a 100.64 address can otherwise win before a
// working LAN address and send encrypted packets back into the tunnel.
func usableUnderlayIP(ip net.IP, meshIP string) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.Equal(net.ParseIP(meshIP))
}
