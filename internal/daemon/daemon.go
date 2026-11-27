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
	"runtime"
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
	IfName   string // requested TUN name; platform default when empty
	Verbose  bool

	// TUN, when set, is used instead of creating one: mobile VPN APIs
	// hand us an already established tunnel device. The daemon takes
	// ownership and closes it on Down.
	TUN tun.Device
	// ManagedIface says the daemon configures the OS interface itself.
	// Mobile platforms configure it through their VPN APIs instead.
	ManagedIface bool
	// LocalDNS uses a narrow userspace endpoint rather than privileged OS port53.
	LocalDNS bool
}

// Daemon is the running node.
type Daemon struct {
	cfg         Config
	state       *State
	lifecycleMu sync.Mutex
	loops       sync.WaitGroup
	applyMu     sync.Mutex // serialize peer replacement and path refresh
	deviceMu    sync.RWMutex

	mu             sync.Mutex
	status         string // down, preparing, joining, waiting-approval, up, key-expired
	interfaceName  string // actual kernel TUN name; protected by mu
	closed         bool   // permanent disposal boundary; protected by mu
	stopGeneration uint64 // Down retires queued local Up workers; protected by mu
	// joinMu serializes join and rejoin: the control-plane identity
	// (node id, token, auth key, control client) swaps as one step,
	// never concurrently from two loops.
	joinMu   sync.Mutex
	dev      *device.Device
	tun      tun.Device
	bind     *bind.Bind
	cc       *controlclient.Client
	relay    *controlclient.Relay
	localDNS *dns.Tunnel
	dns      *dns.Server
	subnets  *ifacectl.SubnetRoutes
	serving  *ifacectl.Serving
	cancel   context.CancelFunc

	mapChanged  chan struct{} // wake local authorization-expiry guard
	lastMap     *netmap.Map
	approvalURL string
	dnsErr      string
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
		cfg.IfName = DefaultInterfaceName()
	}
	if cfg.ManagedIface {
		if err := validateManagedInterface(cfg.IfName, runtime.GOOS); err != nil {
			return nil, err
		}
	}
	st, err := LoadState(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	return &Daemon{
		cfg:        cfg,
		state:      st,
		status:     "down",
		mapChanged: make(chan struct{}, 1),
		stunTx:     map[stun.TxID]time.Time{},
		peers:      map[string]*peerTrack{},
	}, nil
}

// State returns the current persisted state.
func (d *Daemon) State() State {
	d.mu.Lock()
	defer d.mu.Unlock()
	snapshot := *d.state
	snapshot.AdvertiseRoutes = append([]string(nil), d.state.AdvertiseRoutes...)
	return snapshot
}

// SetRouting configures exit-node and subnet-routing roles. advertiseExit
// offers this node as an exit node, routes are subnet CIDRs it serves,
// useExit is a peer name or mesh IP to send internet traffic through.
// Persists; takes effect on the next Up.
func (d *Daemon) SetRouting(advertiseExit bool, routes []string, useExit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("node is closed")
	}
	if d.status != "down" {
		return fmt.Errorf("routing changes apply while down; node is %s", d.status)
	}
	previousExit, previousRoutes, previousUse := d.state.ExitNode, d.state.AdvertiseRoutes, d.state.UseExitNode
	d.state.ExitNode = advertiseExit
	d.state.AdvertiseRoutes = append([]string(nil), routes...)
	d.state.UseExitNode = useExit
	if err := d.state.Save(d.cfg.StateDir); err != nil {
		d.state.ExitNode, d.state.AdvertiseRoutes, d.state.UseExitNode = previousExit, previousRoutes, previousUse
		return err
	}
	return nil
}

// checkStartupRoutingLocked rejects unsupported roles before enrollment or
// host changes. The caller holds d.mu.
func (d *Daemon) checkStartupRoutingLocked() error {
	if d.state.UseExitNode != "" || d.state.ExitNode || (len(d.state.AdvertiseRoutes) > 0 && (!d.cfg.ManagedIface || runtime.GOOS != "linux")) {
		return fmt.Errorf("exit routing and non-Linux/mobile subnet serving are not supported; use mesh-only mode or managed Linux private IPv4 subnets")
	}
	return nil
}

// Up brings the mesh up. server, network and authKey are only needed on the
// first join; after that the saved state is enough.
func (d *Daemon) Up(server, network, nodeName, authKey string) error {
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	return d.upLocked(server, network, nodeName, authKey, nil)
}

// upLocked requires lifecycleMu. A local worker must still belong to the
// generation it acknowledged; direct callers do not supply a generation.
func (d *Daemon) upLocked(server, network, nodeName, authKey string, generation *uint64) (err error) {
	d.mu.Lock()
	if generation != nil && *generation != d.stopGeneration {
		d.mu.Unlock()
		return nil
	}
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("node is closed")
	}
	if d.status != "down" {
		d.mu.Unlock()
		return fmt.Errorf("already %s", d.status)
	}
	// Publish cancellation with startup admission, before disk or control I/O.
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	d.status = "joining"
	d.dnsErr = ""
	d.lastErr = "" // start a new attempt; keep any errors raised during it
	routingErr := d.checkStartupRoutingLocked()
	d.mu.Unlock()

	if routingErr != nil {
		d.setDown()
		return routingErr
	}
	// A failed teardown remains owned. Never overwrite its retry handle.
	if d.serving != nil {
		if err := d.serving.Close(); err != nil {
			d.setDown()
			return fmt.Errorf("retry serving cleanup: %w", err)
		}
		d.serving = nil
	}
	// Status can poll during asynchronous startup. Publish identity/config
	// writes under the same lock its snapshot uses.
	if err := func() error {
		d.mu.Lock()
		defer d.mu.Unlock()
		if err := d.state.validateJoinTarget(server, network); err != nil {
			return err
		}
		if err := d.state.EnsureKey(d.cfg.StateDir); err != nil {
			return err
		}
		previousServer, previousNetwork, previousName := d.state.Server, d.state.Network, d.state.NodeName
		published := false
		defer func() {
			if !published {
				d.state.Server, d.state.Network, d.state.NodeName = previousServer, previousNetwork, previousName
			}
		}()
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
			return err
		}

		published = true

		return nil
	}(); err != nil {
		d.setDown()
		return err
	}

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
	d.mu.Lock()
	d.lastMap = m
	d.mu.Unlock()
	if d.cfg.ManagedIface {
		if err := ifacectl.Configure(d.actualInterfaceName(), m.Self.IPv4, MTU); err != nil {
			d.stopDevice()
			d.setDown()
			cancel()
			return fmt.Errorf("configure interface: %w (need root or admin?)", err)
		}
	}
	if d.cfg.ManagedIface && runtime.GOOS == "linux" {
		d.subnets = ifacectl.NewSubnetRoutes(d.cfg.IfName)
		if d.subnets == nil {
			d.stopDevice()
			d.setDown()
			cancel()
			return fmt.Errorf("initialize subnet route owner")
		}
	}
	if d.state.ExitNode || len(d.state.AdvertiseRoutes) > 0 {
		var servingErr error
		if d.state.ExitNode || !d.cfg.ManagedIface {
			servingErr = fmt.Errorf("automatic exit/mobile serving is not supported")
		} else {
			d.serving, servingErr = ifacectl.NewServing(d.cfg.IfName)
			if servingErr == nil {
				servingErr = d.serving.Start(d.state.AdvertiseRoutes)
			}
		}
		if servingErr != nil {
			d.stopDevice()
			d.setDown()
			cancel()
			return fmt.Errorf("subnet serving setup: %w", servingErr)
		}
	}
	d.applyMap(m)

	// DNS listener readiness is separate from a locally active tunnel. Mobile
	// installs this address as its resolver and must stop if it is unavailable.
	dnsAddress := m.Self.IPv4
	if d.localDNS != nil {
		dnsAddress = dns.LocalIPv4
	}
	dnsSrv, dnsErr := dns.New(dnsAddress)
	if dnsErr == nil {
		dnsSrv.Update(m)
		if d.localDNS != nil {
			dnsErr = d.localDNS.StartDNS(dnsSrv, MTU)
		} else {
			dnsErr = dnsSrv.Start()
		}
	}
	if dnsErr == nil {
		d.applyMu.Lock()
		d.dns = dnsSrv
		d.applyMu.Unlock()
	} else {
		if dnsSrv != nil {
			dnsSrv.Close()
		}
		d.mu.Lock()
		d.dnsErr = dnsErr.Error()
		d.mu.Unlock()
		log.Printf("mnetd: private DNS listener unavailable; details stay in local status")
	}

	d.mu.Lock()
	d.status = "up"
	if m.Self.KeyExpired() {
		d.status = "key-expired"
	}
	d.mu.Unlock()

	for _, loop := range []func(context.Context){d.mapLoop, d.endpointLoop, d.pathLoop, d.relayLoop, d.expiryLoop} {
		d.loops.Add(1)
		go func(f func(context.Context)) { defer d.loops.Done(); f(ctx) }(loop)
	}
	return nil
}

// join registers and waits for approval when needed.
// postJoin sends one join request.
func (d *Daemon) postJoin(ctx context.Context, authKey string) (*netmap.JoinResponse, error) {
	cc := &controlclient.Client{Base: d.state.Server, NodeID: d.state.NodeID, Token: d.state.NodeToken}
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
		// server verifies the saved node credential before preserving identity.
		log.Printf("mnetd: keyed join failed (%v); retrying as known node", err)
		resp, err = d.postJoin(ctx, "")
	}
	if err != nil {
		return fmt.Errorf("join: %w", err)
	}
	d.mu.Lock()
	d.state.NodeID = resp.NodeID
	d.state.NodeToken = resp.NodeToken
	if authKey != "" {
		d.state.AuthKey = authKey
	}
	if err := d.state.Save(d.cfg.StateDir); err != nil {
		d.mu.Unlock()
		return fmt.Errorf("enrollment accepted but saving its credential failed; fix local state storage and retry before restarting: %w", err)
	}
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
	log.Printf("mnetd: waiting for approval; open the approval URL from local status")
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
		waitContext(ctx, 30*time.Second)
		return
	}
	log.Printf("mnetd: control plane no longer knows this node; re-joining")
	// Keep the credential for an expired but still-known identity.
	// A deleted identity can enroll anew using the saved valid auth key.
	if err := d.joinLocked(ctx, d.state.AuthKey); err != nil {
		log.Printf("mnetd: rejoin: %v", err)
		waitContext(ctx, 10*time.Second)
	}
}

func (d *Daemon) startDevice() error {
	started := false
	defer func() {
		if !started {
			d.mu.Lock()
			d.interfaceName = ""
			d.mu.Unlock()
		}
	}()
	priv, err := base64DecodeKey(d.state.PrivateKey)
	if err != nil {
		return err
	}
	t := d.cfg.TUN
	d.cfg.TUN = nil // ownership moves from pending slot into this device attempt
	if t == nil {
		var err error
		t, err = tun.CreateTUN(d.cfg.IfName, MTU)
		if err != nil {
			return fmt.Errorf("create tun: %w (need root or admin, and wintun.dll on Windows)", err)
		}
	}
	if d.cfg.ManagedIface {
		name, err := kernelInterfaceName(t, d.cfg.IfName, runtime.GOOS)
		if err != nil {
			t.Close()
			return err
		}
		d.mu.Lock()
		d.interfaceName = name
		d.mu.Unlock()
	}
	if d.cfg.LocalDNS {
		wrapped := dns.NewTunnel(t)
		d.localDNS = wrapped
		t = wrapped
	}
	b := bind.New()
	level := device.LogLevelError
	if d.cfg.Verbose {
		level = device.LogLevelVerbose
	}
	logger := device.NewLogger(level, "mnetd: ")
	dev := device.NewDevice(t, b, logger)
	d.deviceMu.Lock()
	d.tun, d.bind, d.dev = t, b, dev
	d.deviceMu.Unlock()

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
	started = true
	return nil
}

func (d *Daemon) stopDevice() {
	if pending := d.cfg.TUN; pending != nil {
		d.cfg.TUN = nil
		_ = pending.Close()
	}
	defer func() { d.mu.Lock(); d.interfaceName = ""; d.mu.Unlock() }()
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	if d.serving != nil {
		if err := d.serving.Close(); err != nil {
			d.setErr(fmt.Errorf("serving cleanup: %w", err))
		} else {
			d.serving = nil
		}
	}
	if d.subnets != nil {
		if err := d.subnets.Close(); err != nil {
			d.setErr(fmt.Errorf("subnet cleanup: %w", err))
		}
		d.subnets = nil
	}
	d.deviceMu.Lock()
	defer d.deviceMu.Unlock()
	if d.dev != nil {
		d.dev.Close()
		d.dev = nil
	}
	if d.dns != nil {
		d.dns.Close()
		d.dns = nil
	}
	d.localDNS = nil
}

// Close permanently disposes this node. Unlike Down, later Prepare, Up and
// tunnel injection cannot revive it. Mark first so a concurrent startup either
// sees closed before publishing cancellation or is canceled by Down.
func (d *Daemon) Close() {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	d.Down()
}

// Down tears the mesh down.
func (d *Daemon) Down() {
	d.mu.Lock()
	d.stopGeneration++
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	// Up may have published its cancellation function after the first read.
	d.mu.Lock()
	cancel = d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	d.loops.Wait()
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

func waitContext(ctx context.Context, duration time.Duration) {
	t := time.NewTimer(duration)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
func (d *Daemon) currentMap() *netmap.Map {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastMap
}
