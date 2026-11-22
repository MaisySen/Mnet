package netmap

import (
	"encoding/base64"
	"time"
)

// EndpointCapability is a compatibility declaration, not remote attestation,
// membership, a readiness lease, or permission for subnet/exit forwarding.
// V1 is one staged exact-pair IPv4 endpoint mode, not the whole Mnet product.
type EndpointCapability struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"`
	MTU     int    `json:"mtu"`
}

const EndpointModeV1 = "mnet-ipv4-endpoint-v1"

func (c EndpointCapability) Supported() bool {
	return c == (EndpointCapability{Version: 1, Mode: EndpointModeV1, MTU: 1280})
}

// EndpointProfile is a current-credential-authenticated peer declaration. The
// Store supplies identity/revision/lifetime, never accepts those from the wire.
// It is volatile: restart withdraws it; it does not renew original authority.
type EndpointProfile struct {
	Capability    EndpointCapability `json:"capability"`
	NetworkID     string             `json:"network_id"`
	NodeID        string             `json:"node_id"`
	PublicKey     string             `json:"public_key"`
	IPv4          string             `json:"ipv4"`
	PublicationID string             `json:"publication_id"`
	Revision      int64              `json:"revision"`
	AccessUntilMS int64              `json:"access_until_ms"`
}

func (p EndpointProfile) Matches(n Node, now time.Time) bool {
	raw, e := base64.RawURLEncoding.Strict().DecodeString(p.PublicationID)
	return e == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == p.PublicationID && p.Capability.Supported() && p.Revision > 0 && n.Approved && !n.ExitNode && len(n.Routes) == 0 && n.KeyExpiry != nil && p.AccessUntilMS == n.KeyExpiry.UnixMilli() && now.UnixMilli() < p.AccessUntilMS && p.NetworkID == n.NetworkID && p.NodeID == n.ID && p.PublicKey == n.PubKey && p.IPv4 == n.IPv4
}
