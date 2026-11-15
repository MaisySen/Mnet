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
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ACL       *ACLPolicy `json:"acl,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ACLPolicy is the network's access list. Nil or empty means allow all
// (the default for new networks). With at least one rule the default is
// deny: a peer pair only connects when a rule allows it.
type ACLPolicy struct {
	Rules []ACLRule `json:"rules"`
}

// ACLRule allows traffic from src to dst. src matches node names (case
// insensitive) or "*". dst additionally accepts CIDRs for advertised
// subnet routes and "0.0.0.0/0" for exit-node internet access.
type ACLRule struct {
	Src     string `json:"src"`
	Dst     string `json:"dst"`
	Comment string `json:"comment,omitempty"`
}

// Node is one device in a network.
type Node struct {
	ID        string     `json:"id"`
	NetworkID string     `json:"network_id"`
	Name      string     `json:"name"`      // hostname, unique inside the network
	PubKey    string     `json:"pubkey"`    // WireGuard public key, base64
	IPv4      string     `json:"ipv4"`      // assigned 100.64.0.0/10 address
	Endpoints []string   `json:"endpoints"` // observed UDP endpoints, host:port
	Approved  bool       `json:"approved"`
	User      string     `json:"user,omitempty"`   // SSO identity that approved the node
	ExitNode  bool       `json:"exit_node"`        // offers to carry others' internet traffic
	Routes    []string   `json:"routes,omitempty"` // advertised subnet CIDRs
	CreatedAt time.Time  `json:"created_at"`
	LastSeen  time.Time  `json:"last_seen"`
	KeyExpiry *time.Time `json:"key_expiry,omitempty"` // node key stops working after this
}

// KeyExpired reports whether the node's key has passed its expiry.
func (n *Node) KeyExpired() bool {
	return n.KeyExpiry != nil && time.Now().After(*n.KeyExpiry)
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
	// Populated by the server from authenticated request headers, never JSON.
	NodeID    string   `json:"-"`
	NodeToken string   `json:"-"`
	Network   string   `json:"network"` // network name or ID
	Name      string   `json:"name"`    // requested node name (hostname)
	PubKey    string   `json:"pubkey"`
	AuthKey   string   `json:"auth_key,omitempty"`  // optional pre-auth key
	ExitNode  bool     `json:"exit_node,omitempty"` // offer to be an exit node
	Routes    []string `json:"routes,omitempty"`    // subnet CIDRs to advertise
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
