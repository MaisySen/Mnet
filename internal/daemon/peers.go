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
	"strconv"
	"strings"
	"time"

	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/ifacectl"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/stun"
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
}

// applyMap writes the map into the WireGuard device.
func (d *Daemon) applyMap(m *netmap.Map) {
	if d.dev == nil {
		return
	}
	var conf bytes.Buffer
	for _, p := range m.Peers {
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
		if ep := st.currentEndpoint(); ep != "" {
			fmt.Fprintf(&conf, "endpoint=%s\n", ep)
		}
	}
	if err := d.dev.IpcSetOperation(&conf); err != nil {
		d.setErr(fmt.Errorf("apply map: %w", err))
		return
	}
	d.lastMap = m
	if d.dns != nil {
		d.dns.Update(m)
	}
}

func (st *peerTrack) currentEndpoint() string {
	if st.onRelay || len(st.node.Endpoints) == 0 {
		return bind.RelayPrefix + st.node.PubKey
	}
	return st.node.Endpoints[st.cycle%len(st.node.Endpoints)]
}

// applyExitNode switches the underlay default route into the tunnel so all
// internet traffic leaves through the selected exit node. Kernel routing,
// skipped on managed (mobile) interfaces where the OS VPN API owns routes.
func (d *Daemon) applyExitNode(m *netmap.Map) error {
	var target *netmap.Node
	for i, p := range m.Peers {
		if p.ExitNode && (p.Name == d.state.UseExitNode || p.IPv4 == d.state.UseExitNode) {
			target = &m.Peers[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("exit node %q not found in map", d.state.UseExitNode)
	}
	if d.cfg.ManagedIface {
		return nil
	}
	gw, dev, err := ifacectl.DefaultRoute()
	if err != nil {
		return err
	}
	d.peersMu.Lock()
	var ep string
	if st := d.peers[target.PubKey]; st != nil {
		ep = st.currentEndpoint()
	}
	d.peersMu.Unlock()
	if ep != "" && !strings.HasPrefix(ep, bind.RelayPrefix) {
		if err := ifacectl.HostRoute(ep, gw); err != nil {
			return err
		}
	}
	if err := ifacectl.HostRouteURL(d.state.Server, gw); err != nil {
		log.Printf("mnetd: control host route: %v", err)
	}
	if err := ifacectl.DefaultRouteVia(d.cfg.IfName); err != nil {
		return err
	}
	log.Printf("mnetd: internet traffic exits via %s (%s), underlay gateway %s dev %s", target.Name, target.IPv4, gw, dev)
	return nil
}

// mapLoop long polls the control plane for new maps.
func (d *Daemon) mapLoop(ctx context.Context) {
	for {
		var since int64
		if d.lastMap != nil {
			since = d.lastMap.Version
		}
		m, err := d.client().WaitMap(ctx, since)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, controlclient.ErrUnauthorized) {
				d.rejoin(ctx)
				continue
			}
			time.Sleep(2 * time.Second)
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
		d.discoverEndpoints()
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

func (d *Daemon) discoverEndpoints() {
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
	time.Sleep(1200 * time.Millisecond)
	d.stunMu.Lock()
	if d.stunResult != "" {
		eps[d.stunResult] = true
	}
	d.stunMu.Unlock()

	// Local LAN addresses help peers on the same network.
	ifaces, _ := net.InterfaceAddrs()
	for _, a := range ifaces {
		ip, _, err := net.ParseCIDR(a.String())
		if err != nil {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() && !ip4.IsLinkLocalUnicast() {
			eps[fmt.Sprintf("%s:%d", ip4, port)] = true
		}
	}

	out := make([]string, 0, len(eps))
	for e := range eps {
		out = append(out, e)
	}
	if err := d.client().ReportEndpoints(context.Background(), out); err != nil {
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
	out := map[string]*peerStats{}
	if d.dev == nil {
		return out
	}
	var buf bytes.Buffer
	if err := d.dev.IpcGetOperation(&buf); err != nil {
		return out
	}
	var cur *peerStats
	var curKey string
	for _, line := range strings.Split(buf.String(), "\n") {
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
		case "latest_handshake":
			if cur != nil {
				cur.handshake, _ = strconv.ParseInt(v, 10, 64)
			}
		case "transfer_rx":
			if cur != nil {
				cur.rx, _ = strconv.ParseInt(v, 10, 64)
			}
		case "transfer_tx":
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
		stats := d.stats()
		now := time.Now()
		changed := false
		d.peersMu.Lock()
		for pk, st := range d.peers {
			s := stats[pk]
			healthy := s != nil && s.handshake > 0 && now.Unix()-s.handshake < 90
			if healthy {
				// A working direct path leaves relay mode behind.
				if st.onRelay && s != nil && !strings.HasPrefix(s.endpoint, bind.RelayPrefix) && s.endpoint != "" {
					st.onRelay = false
					changed = true
				}
				continue
			}
			if !st.onRelay {
				// Rotate candidates, then give up to the relay.
				if len(st.node.Endpoints) == 0 || now.Sub(st.lastSwitch) > 30*time.Second {
					st.onRelay = true
					st.lastSwitch = now
					changed = true
				} else if now.Sub(st.lastSwitch) > 5*time.Second {
					st.cycle++
					st.lastSwitch = now
					changed = true
				}
			} else if len(st.node.Endpoints) > 0 && now.Sub(st.lastDirectTry) > 90*time.Second {
				// Retry direct now and then; roaming keeps it if it works.
				st.onRelay = false
				st.lastSwitch = now
				st.lastDirectTry = now
				changed = true
			}
		}
		d.peersMu.Unlock()
		if changed && d.lastMap != nil {
			d.applyMap(d.lastMap)
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
			time.Sleep(time.Second)
			continue
		}
		r, err := d.client().DialRelay(ctx, d.state.PublicKey, d.bind.Inject)
		if err != nil {
			if errors.Is(err, controlclient.ErrUnauthorized) {
				d.rejoin(ctx)
				continue
			}
			log.Printf("mnetd: relay dial failed: %v (retrying)", err)
			time.Sleep(3 * time.Second)
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
		time.Sleep(2 * time.Second)
	}
}

// PeerStatuses renders the current peer table for the CLI.
func (d *Daemon) PeerStatuses() []PeerStatus {
	stats := d.stats()
	now := time.Now()
	var out []PeerStatus
	if d.lastMap == nil {
		return out
	}
	for _, p := range d.lastMap.Peers {
		ps := PeerStatus{
			Name:        p.Name,
			IPv4:        p.IPv4,
			FQDN:        p.FQDN(d.lastMap.Network.Name),
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
