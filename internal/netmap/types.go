// Package netmap holds the types shared between the Mnet
// control plane and its clients. A network map is the full set of nodes
// a client should connect to, the same idea as Tailscale's tailcfg.MapResponse.
package netmap

import "time"

const (
	// VPNBase is the first usable address we hand out from the CGNAT
	// carrier range, the same 100.64.0.0/10 block Tailscale uses.
	VPNBase    = "100.64.0.1"
	VPNNetwork = "100.64.0.0/10"
	// DNSSuffix is the Magic DNS suffix every node name lives under.
	DNSSuffix = "mnet"
)

// Network is one isolated mesh. Nodes from different networks never see
// each other.
type Network struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Node is one device in a network.
type Node struct {
	ID        string    `json:"id"`
	NetworkID string    `json:"network_id"`
	Name      string    `json:"name"`      // hostname, unique inside the network
	PubKey    string    `json:"pubkey"`    // WireGuard public key, base64
	IPv4      string    `json:"ipv4"`      // assigned 100.64.0.0/10 address
	Endpoints []string  `json:"endpoints"` // observed UDP endpoints, host:port
	Approved  bool      `json:"approved"`
	ExitNode  bool      `json:"exit_node"`        // offers to carry other nodes' internet traffic
	Routes    []string  `json:"routes,omitempty"` // advertised subnet CIDRs
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
}

// FQDN returns the Magic DNS name of the node, without a trailing dot.
func (n *Node) FQDN(networkName string) string {
	return n.Name + "." + networkName + "." + DNSSuffix
}

// AuthKey is a pre-authorised join key for headless devices.
type AuthKey struct {
	ID        string     `json:"id"`
	NetworkID string     `json:"network_id"`
	Key       string     `json:"key"`
	Note      string     `json:"note"`
	Reusable  bool       `json:"reusable"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Uses      int        `json:"uses"`
}

// Map is the network map a client consumes.
type Map struct {
	Version   int64     `json:"version"`
	Network   Network   `json:"network"`
	Self      Node      `json:"self"`
	Peers     []Node    `json:"peers"`
	RelayURL  string    `json:"relay_url"` // WebSocket relay endpoint
	Generated time.Time `json:"generated"`
}

// JoinRequest is what a client sends to join a network.
type JoinRequest struct {
	Network  string   `json:"network"` // network name or ID
	Name     string   `json:"name"`    // requested node name (hostname)
	PubKey   string   `json:"pubkey"`
	AuthKey  string   `json:"auth_key,omitempty"`  // optional pre-auth key
	ExitNode bool     `json:"exit_node,omitempty"` // offer to be an exit node
	Routes   []string `json:"routes,omitempty"`    // subnet CIDRs to advertise
}

// JoinResponse is returned immediately after a join request.
type JoinResponse struct {
	NodeID      string `json:"node_id"`
	NodeToken   string `json:"node_token"` // authenticates all later node calls
	Approved    bool   `json:"approved"`
	ApprovalURL string `json:"approval_url,omitempty"` // shown when approval is pending
}

// JoinStatus is the answer to polling while approval is pending.
type JoinStatus struct {
	Approved bool   `json:"approved"`
	IPv4     string `json:"ipv4,omitempty"`
}

// EndpointsUpdate reports the node's current candidate endpoints.
type EndpointsUpdate struct {
	Endpoints []string `json:"endpoints"`
}

// APIError is the common error body.
type APIError struct {
	Error string `json:"error"`
}
