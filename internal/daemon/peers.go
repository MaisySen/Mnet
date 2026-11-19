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
	Name           string `json:"name"`
	IPv4           string `json:"ipv4"`
	FQDN           string `json:"fqdn"`
	PubKeyShort    string `json:"pubkey_short"`
	Endpoint       string `json:"endpoint"`
	Relay          bool   `json:"relay"`
	LastHandshake  int64  `json:"last_handshake_ago_sec"`
	HandshakeState string `json:"handshake_state,omitempty"`
	Rx             int64  `json:"rx_bytes"`
	Tx             int64  `json:"tx_bytes"`
	Online         bool   `json:"online"`
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
	endpointFilter := newEndpointRouteFilter(m.Peers)
	for _, p := range m.Peers {
		p.Endpoints = endpointFilter.safe(p.Endpoints)
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
	return newEndpointRouteFilter(peers).safe(endpoints)
}

// Build once per map, not once per peer. Membership requires at most 33 IPv4
// or 129 IPv6 prefix-length probes, regardless of the number of routed peers.
type endpointRouteFilter struct {
	prefixes       map[netip.Prefix]struct{}
	v4Bits, v6Bits []int
}

func newEndpointRouteFilter(peers []netmap.Node) endpointRouteFilter {
	f := endpointRouteFilter{prefixes: make(map[netip.Prefix]struct{})}
	var v4Seen [33]bool
	var v6Seen [129]bool
	add := func(p netip.Prefix) {
		p = p.Masked()
		f.prefixes[p] = struct{}{}
		bits := p.Bits()
		if p.Addr().Is4() {
			if !v4Seen[bits] {
				f.v4Bits = append(f.v4Bits, bits)
				v4Seen[bits] = true
			}
		} else if !v6Seen[bits] {
			f.v6Bits = append(f.v6Bits, bits)
			v6Seen[bits] = true
		}
	}
	add(netip.MustParsePrefix("100.64.0.0/10"))
	for _, peer := range peers {
		for _, raw := range peer.Routes {
			if p, err := netip.ParsePrefix(raw); err == nil {
				add(p)
			}
		}
	}
	return f
}

func (f endpointRouteFilter) contains(addr netip.Addr) bool {
	// Preserve Prefix.Contains semantics: a zoned address never matches a
	// prefix, and IPv4-mapped endpoints are checked as their IPv4 address.
	addr = addr.Unmap()
	if addr.Zone() != "" {
		return false
	}
	bits := f.v6Bits
	if addr.Is4() {
		bits = f.v4Bits
	}
	for _, n := range bits {
		if _, blocked := f.prefixes[netip.PrefixFrom(addr, n).Masked()]; blocked {
			return true
		}
	}
	return false
}

func (f endpointRouteFilter) safe(endpoints []string) []string {
	var safe []string
	for _, endpoint := range endpoints {
		addr, err := netip.ParseAddrPort(endpoint)
		if err != nil {
			continue
		}
		if !f.contains(addr.Addr()) {
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
// A received authorization denial must withdraw the old dataplane map,
// not preserve it as though only connectivity had failed.
func (d *Daemon) denyMap(m *netmap.Map) {
	denied := *m
	denied.Self.Approved = false
	denied.Peers = nil
	d.applyMap(&denied)
	d.mu.Lock()
	if d.status == "up" {
		d.status = "waiting-approval"
	}
	d.mu.Unlock()
}

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
			if errors.Is(err, controlclient.ErrForbidden) {
				d.denyMap(&netmap.Map{})
				waitContext(ctx, 2*time.Second)
				continue
			}
			if errors.Is(err, controlclient.ErrUnauthorized) {
				// Revocation must stop existing dataplane authorization before
				// waiting for a fresh join/approval.
				d.denyMap(&netmap.Map{})
				d.rejoin(ctx)
				continue
			}
			waitContext(ctx, 2*time.Second)
			continue
		}
		if m.Self.Approved {
			d.applyMap(m)
		} else {
			d.denyMap(m)
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

	req, txid := stun.NewRequest()
	d.startStunProbe(txid, time.Now())
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

// startStunProbe prunes even when every STUN server is silent. Waiting for
// onStun to prune leaves unanswered transactions growing on offline networks.
func (d *Daemon) startStunProbe(txid stun.TxID, now time.Time) {
	d.stunMu.Lock()
	defer d.stunMu.Unlock()
	d.expireStunLocked(now)
	d.stunResult = ""
	d.stunTx[txid] = now
}

// expireStunLocked requires stunMu.
func (d *Daemon) expireStunLocked(now time.Time) {
	for txid, at := range d.stunTx {
		if now.Sub(at) > 10*time.Second {
			delete(d.stunTx, txid)
		}
	}
}

// onStun handles intercepted STUN responses from the bind.
func (d *Daemon) onStun(msg []byte) {
	d.stunMu.Lock()
	defer d.stunMu.Unlock()
	d.expireStunLocked(time.Now())
	for txid := range d.stunTx {
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
			if errors.Is(err, controlclient.ErrForbidden) {
				d.denyMap(&netmap.Map{})
				waitContext(ctx, 3*time.Second)
				continue
			}
			if errors.Is(err, controlclient.ErrUnauthorized) {
				// Revocation must stop existing dataplane authorization before
				// waiting for a fresh join/approval.
				d.denyMap(&netmap.Map{})
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
	return renderPeerStatuses(d.currentMap(), d.stats(), time.Now())
}

// Render evidence, not assumed reachability. A missing device/stat entry has
// no handshake age. Future timestamps after a wall-clock step cannot be called
// online. Invalid map keys were never usable WG peers and must not panic here.
func renderPeerStatuses(m *netmap.Map, stats map[string]*peerStats, now time.Time) []PeerStatus {
	var out []PeerStatus
	if m == nil {
		return out
	}
	for _, p := range m.Peers {
		raw, err := base64.StdEncoding.Strict().DecodeString(p.PubKey)
		if err != nil || len(raw) != 32 || base64.StdEncoding.EncodeToString(raw) != p.PubKey {
			continue
		}
		ps := PeerStatus{Name: p.Name, IPv4: p.IPv4, FQDN: p.FQDN(m.Network.Name), PubKeyShort: p.PubKey[:12] + "...", LastHandshake: -1, HandshakeState: "unavailable"}
		if s := stats[p.PubKey]; s != nil {
			ps.Endpoint = s.endpoint
			ps.Relay = strings.HasPrefix(s.endpoint, bind.RelayPrefix)
			ps.Rx, ps.Tx = s.rx, s.tx
			switch {
			case s.handshake <= 0:
				ps.HandshakeState = "never"
			case s.handshake > now.Unix():
				ps.HandshakeState = "clock-skew"
			default:
				ps.LastHandshake = now.Unix() - s.handshake
				ps.Online = ps.LastHandshake < 180
				ps.HandshakeState = "stale"
				if ps.Online {
					ps.HandshakeState = "recent"
				}
			}
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
