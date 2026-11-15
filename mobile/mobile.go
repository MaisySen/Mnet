// Package mobile exposes the Mnet node engine to phone apps through
// gomobile bind. The app establishes the OS tunnel itself, then hands it
// to the node: as a file descriptor on Android (Start), or as a
// PacketFlow on iOS (StartFlow).
//
// Build for Android:
//
//	gomobile bind -target android -androidapi 24 -o mnet.aar fintex.au/mnet/mobile
package mobile

import (
	"encoding/json"
	"fmt"

	"fintex.au/mnet/internal/daemon"
)

// Node is one Mnet node on a phone. All methods are safe to call from
// the UI thread; blocking work happens on daemon goroutines.
type Node struct {
	d *daemon.Daemon
}

// NewNode makes a node that stores keys and join state in stateDir.
// The directory must be private to the app.
func NewNode(stateDir string) (*Node, error) {
	if stateDir == "" {
		return nil, fmt.Errorf("state dir required")
	}
	d, err := daemon.New(daemon.Config{StateDir: stateDir, ManagedIface: false})
	if err != nil {
		return nil, err
	}
	return &Node{d: d}, nil
}

// Prepare joins the network and returns the assigned mesh IPv4 address,
// without touching the OS VPN interface. The app uses the address to
// configure the tunnel builder, establishes it, then calls Start (or
// StartFlow). server, network and authKey are only needed on first join.
// If the node needs admin approval, Prepare blocks until it arrives.
func (n *Node) Prepare(server, network, name, authKey string) (string, error) {
	return n.d.Prepare(server, network, name, authKey)
}

// SetUseExitNode routes the device's internet traffic through the named
// peer (name or mesh IP). Call before Prepare; pass an empty string to
// clear. The app must also add a default route to its tunnel builder.
func (n *Node) SetUseExitNode(peer string) error {
	return n.d.SetRouting(false, nil, peer)
}

// Status returns the daemon status as JSON: state, mesh IP, peers with
// paths and handshake times. Never blocks on the network.
func (n *Node) Status() string {
	b, err := json.Marshal(n.d.Status())
	if err != nil {
		return `{"status":"error"}`
	}
	return string(b)
}

// Stop tears the mesh down and closes the tunnel handed to Start.
func (n *Node) Stop() {
	n.d.Down()
}

// MeshIPv4 returns the assigned mesh address after a successful Start,
// or an empty string.
func (n *Node) MeshIPv4() string {
	return n.d.Status().IPv4
}
