package daemon

// Mobile entry points. Phone VPN APIs own the tunnel device and the
// interface configuration, so the daemon joins and fetches the assigned
// address first (Prepare), then comes up on the injected tunnel (Up with
// Config.TUN set, ManagedIface false).

import (
	"context"
	"fmt"
	"time"

	"golang.zx2c4.com/wireguard/tun"

	"fintex.au/mnet/internal/controlclient"
)

// SetTUN injects an already established tunnel device. Valid while down;
// the daemon takes ownership and closes it on Down.
func (d *Daemon) SetTUN(t tun.Device) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != "down" {
		return fmt.Errorf("already %s", d.status)
	}
	d.cfg.TUN = t
	return nil
}

// Prepare joins the network and returns the assigned mesh IPv4 address
// without starting any device. Mobile VPN builders need the address
// before they can establish the tunnel; call Up afterwards. Joining is
// idempotent, so the later Up re-joins harmlessly.
func (d *Daemon) Prepare(server, network, nodeName, authKey string) (string, error) {
	d.mu.Lock()
	if d.status != "down" {
		d.mu.Unlock()
		return "", fmt.Errorf("already %s", d.status)
	}
	d.mu.Unlock()

	if err := d.state.EnsureKey(d.cfg.StateDir); err != nil {
		return "", err
	}
	if server != "" {
		d.state.Server = server
	}
	if network != "" {
		d.state.Network = network
	}
	if nodeName != "" {
		d.state.NodeName = nodeName
	}
	if d.state.Server == "" || d.state.Network == "" {
		return "", fmt.Errorf("need server and network for the first join")
	}
	if err := d.state.Save(d.cfg.StateDir); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := d.join(ctx, authKey); err != nil {
		return "", err
	}
	cc := &controlclient.Client{Base: d.state.Server, NodeID: d.state.NodeID, Token: d.state.NodeToken}
	m, err := cc.WaitMap(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("first map: %w", err)
	}
	return m.Self.IPv4, nil
}
