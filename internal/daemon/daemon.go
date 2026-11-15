// Package daemon is the Mnet node engine: it owns the WireGuard device,
// keeps the network map applied, discovers endpoints, picks direct or
// relayed paths, and serves the CLI's local API.
package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"

	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/dns"
	"fintex.au/mnet/internal/ifacectl"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/stun"
)

// MTU fits WireGuard inside one ordinary internet frame.
const MTU = 1280

// Config starts a daemon.
type Config struct {
	StateDir string
	IfName   string // TUN name, default mnet0
	Verbose  bool

	// TUN, when set, is used instead of creating one: mobile VPN APIs
	// hand us an already established tunnel device. The daemon takes
	// ownership and closes it on Down.
	TUN tun.Device
	// ManagedIface says the daemon configures the OS interface itself.
	// Mobile platforms configure it through their VPN APIs instead.
	ManagedIface bool
}

// Daemon is the running node.
type Daemon struct {
	cfg   Config
	state *State

	mu     sync.Mutex
	status string // down, joining, waiting-approval, up
	// joinMu serializes join and rejoin: the control-plane identity
	// (node id, token, auth key, control client) swaps as one step,
	// never concurrently from two loops.
	joinMu sync.Mutex
	dev    *device.Device
	tun    tun.Device
	bind   *bind.Bind
	cc     *controlclient.Client
	relay  *controlclient.Relay
	dns    *dns.Server
	cancel context.CancelFunc

	lastMap     *netmap.Map
	approvalURL string
	lastErr     string

	peersMu sync.Mutex
	peers   map[string]*peerTrack

	// STUN bookkeeping.
	stunMu     sync.Mutex
	stunTx     map[stun.TxID]time.Time
	stunResult string
}

// New makes a stopped daemon.
func New(cfg Config) (*Daemon, error) {
	if cfg.IfName == "" {
		cfg.IfName = "mnet0"
	}
	st, err := LoadState(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	return &Daemon{
		cfg:    cfg,
		state:  st,
		status: "down",
		stunTx: map[stun.TxID]time.Time{},
		peers:  map[string]*peerTrack{},
	}, nil
}

// State returns the current persisted state.
func (d *Daemon) State() State { return *d.state }

// SetRouting configures exit-node and subnet-routing roles. advertiseExit
// offers this node as an exit node, routes are subnet CIDRs it serves,
// useExit is a peer name or mesh IP to send internet traffic through.
// Persists; takes effect on the next Up.
func (d *Daemon) SetRouting(advertiseExit bool, routes []string, useExit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != "down" {
		return fmt.Errorf("routing changes apply while down; node is %s", d.status)
	}
	d.state.ExitNode = advertiseExit
	d.state.AdvertiseRoutes = routes
	d.state.UseExitNode = useExit
	return d.state.Save(d.cfg.StateDir)
}

// Up brings the mesh up. server, network and authKey are only needed on the
// first join; after that the saved state is enough.
func (d *Daemon) Up(server, network, nodeName, authKey string) error {
	d.mu.Lock()
	if d.status != "down" {
		d.mu.Unlock()
		return fmt.Errorf("already %s", d.status)
	}
	d.status = "joining"
	d.mu.Unlock()

	if err := d.state.EnsureKey(d.cfg.StateDir); err != nil {
		d.setDown()
		return err
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
		d.setDown()
		return fmt.Errorf("need --server and --network for the first join")
	}
	if d.state.NodeName == "" {
		host, _ := os.Hostname()
		if host == "" {
			host = "node"
		}
		d.state.NodeName = host
	}
	if err := d.state.Save(d.cfg.StateDir); err != nil {
		d.setDown()
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()

	if err := d.join(ctx, authKey); err != nil {
		d.setDown()
		cancel()
		return err
	}

	d.cc = &controlclient.Client{
		Base:   d.state.Server,
		NodeID: d.state.NodeID,
		Token:  d.state.NodeToken,
	}

	if err := d.startDevice(); err != nil {
		d.setDown()
		cancel()
		return err
	}

	// First map before the interface gets its address.
	m, err := d.cc.WaitMap(ctx, 0)
	if err != nil {
		d.stopDevice()
		d.setDown()
		cancel()
		return fmt.Errorf("first map: %w", err)
	}
	d.lastMap = m
	if d.state.ExitNode || len(d.state.AdvertiseRoutes) > 0 {
		if err := ifacectl.EnableServing(d.state.AdvertiseRoutes); err != nil {
			log.Printf("mnetd: serving setup: %v", err)
		}
	}
	if d.cfg.ManagedIface {
		if err := ifacectl.Configure(d.cfg.IfName, m.Self.IPv4, MTU); err != nil {
			d.stopDevice()
			d.setDown()
			cancel()
			return fmt.Errorf("configure interface: %w (need root or admin?)", err)
		}
	}
	d.applyMap(m)
	if d.state.UseExitNode != "" {
		if err := d.applyExitNode(m); err != nil {
			log.Printf("mnetd: exit node: %v", err)
		}
	}

	// Magic DNS on our own mesh address.
	if dnsSrv, err := dns.New(m.Self.IPv4); err == nil {
		d.dns = dnsSrv
		dnsSrv.Update(m)
		go dnsSrv.Serve()
	} else {
		log.Printf("mnetd: magic DNS off: %v", err)
	}

	d.mu.Lock()
	d.status = "up"
	d.mu.Unlock()

	go d.mapLoop(ctx)
	go d.endpointLoop(ctx)
	go d.pathLoop(ctx)
	go d.relayLoop(ctx)
	return nil
}

// join registers and waits for approval when needed.
// postJoin sends one join request.
func (d *Daemon) postJoin(ctx context.Context, authKey string) (*netmap.JoinResponse, error) {
	cc := &controlclient.Client{Base: d.state.Server}
	return cc.Join(ctx, netmap.JoinRequest{
		Network:  d.state.Network,
		Name:     d.state.NodeName,
		PubKey:   d.state.PublicKey,
		AuthKey:  authKey,
		ExitNode: d.state.ExitNode,
		Routes:   d.state.AdvertiseRoutes,
	})
}

func (d *Daemon) join(ctx context.Context, authKey string) error {
	d.joinMu.Lock()
	defer d.joinMu.Unlock()
	return d.joinLocked(ctx, authKey)
}

// joinLocked joins with joinMu already held (rejoin's path).
func (d *Daemon) joinLocked(ctx context.Context, authKey string) error {
	resp, err := d.postJoin(ctx, authKey)
	if err != nil && authKey != "" && d.state.NodeID != "" {
		// One-time auth keys are consumed by the first join (Prepare on
		// mobile), so the re-join in Up comes back 403. A node the
		// control plane still knows re-joins fine without a key: the
		// server matches the public key and keeps its approval.
		log.Printf("mnetd: keyed join failed (%v); retrying as known node", err)
		resp, err = d.postJoin(ctx, "")
	}
	if err != nil {
		return fmt.Errorf("join: %w", err)
	}
	d.state.NodeID = resp.NodeID
	d.state.NodeToken = resp.NodeToken
	if authKey != "" {
		d.state.AuthKey = authKey
	}
	if err := d.state.Save(d.cfg.StateDir); err != nil {
		return err
	}
	d.mu.Lock()
	d.cc = &controlclient.Client{
		Base:   d.state.Server,
		NodeID: d.state.NodeID,
		Token:  d.state.NodeToken,
	}
	d.mu.Unlock()
	if resp.Approved {
		return nil
	}
	d.mu.Lock()
	d.status = "waiting-approval"
	d.approvalURL = resp.ApprovalURL
	d.mu.Unlock()
	log.Printf("mnetd: waiting for approval: %s", resp.ApprovalURL)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
		st, err := (&controlclient.Client{Base: d.state.Server}).JoinStatus(ctx, resp.NodeID, resp.NodeToken)
		if err != nil {
			continue
		}
		if st.Approved {
			return nil
		}
	}
}

// client returns the current control-plane client. join swaps it
// under d.mu, so the loops must read it the same way.
func (d *Daemon) client() *controlclient.Client {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cc
}

// rejoin registers the node again after the control plane forgot it
// (state wipe on the server, deleted node). Uses the saved auth key.
func (d *Daemon) rejoin(ctx context.Context) {
	d.joinMu.Lock()
	defer d.joinMu.Unlock()
	if d.state.AuthKey == "" {
		log.Printf("mnetd: control plane no longer knows this node and no auth key is saved; re-run `mnetd -up -auth-key ...`")
		time.Sleep(30 * time.Second)
		return
	}
	log.Printf("mnetd: control plane no longer knows this node; re-joining")
	d.state.NodeID = ""
	d.state.NodeToken = ""
	if err := d.joinLocked(ctx, d.state.AuthKey); err != nil {
		log.Printf("mnetd: rejoin: %v", err)
		time.Sleep(10 * time.Second)
	}
}

func (d *Daemon) startDevice() error {
	priv, err := base64DecodeKey(d.state.PrivateKey)
	if err != nil {
		return err
	}
	t := d.cfg.TUN
	if t == nil {
		var err error
		t, err = tun.CreateTUN(d.cfg.IfName, MTU)
		if err != nil {
			return fmt.Errorf("create tun: %w (need root or admin, and wintun.dll on Windows)", err)
		}
	}
	b := bind.New()
	level := device.LogLevelError
	if d.cfg.Verbose {
		level = device.LogLevelVerbose
	}
	logger := device.NewLogger(level, "mnetd: ")
	dev := device.NewDevice(t, b, logger)
	d.tun, d.bind, d.dev = t, b, dev

	b.SetStunHandler(d.onStun)

	conf := "private_key=" + hex.EncodeToString(priv) + "\nlisten_port=0\n"
	if err := dev.IpcSetOperation(bytes.NewBufferString(conf)); err != nil {
		dev.Close()
		return fmt.Errorf("configure device: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return fmt.Errorf("device up: %w", err)
	}
	return nil
}

func (d *Daemon) stopDevice() {
	if d.dev != nil {
		d.dev.Close()
		d.dev = nil
	}
	if d.dns != nil {
		d.dns.Close()
		d.dns = nil
	}
}

// Down tears the mesh down.
func (d *Daemon) Down() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	d.stopDevice()
	d.setDown()
}

func (d *Daemon) setDown() {
	d.mu.Lock()
	d.status = "down"
	d.cancel = nil
	d.approvalURL = ""
	d.mu.Unlock()
}

func (d *Daemon) setErr(err error) {
	d.mu.Lock()
	d.lastErr = err.Error()
	d.mu.Unlock()
	log.Printf("mnetd: %v", err)
}

func base64DecodeKey(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil, fmt.Errorf("bad stored key")
	}
	return b, nil
}
