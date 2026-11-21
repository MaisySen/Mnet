//go:build mnet_enrollment_development

// Package osownership contains inert planning/transaction models and guarded
// source-only Linux helpers. No live executor, command runner, daemon integration
// or netlink socket is wired. Tests use in-memory fake kernel data only.
package osownership

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

const Protocol = 242
const MTU = 1280

var ErrConflict = errors.New("foreign or replaced networking resource")

// Namespace proof is caller-supplied DATA here. A real executor MUST recover
// this from an open namespace FD and initial-namespace comparison, not config.
type Namespace struct {
	Device, Inode uint64
	Isolated      bool
}
type Binding struct {
	Namespace                      Namespace
	OwnIP, PeerIP, OwnKey, PeerKey string
	SelfExpiry, PeerExpiry         time.Time
	Name, Alias                    string
}
type Link struct {
	Index             int
	Name, Alias, Kind string
}
type Route struct {
	Destination      netip.Prefix
	LinkIndex        int
	Protocol         int
	Table            int
	Scope            string
	Type             string
	Metric, Priority uint32
	KernelOrigin     bool
}
type Address struct {
	Prefix       netip.Prefix
	LinkIndex    int
	KernelOrigin bool
}
type Inventory struct {
	Namespace     Namespace
	Links         []Link
	Routes        []Route
	Addresses     []Address
	LinkUp        map[int]bool
	IPv6Disabled  map[int]bool
	ForeignPolicy bool
	Forwarding    bool
}
type Operation struct {
	Kind        string
	Name, Alias string
	Index       int
	Address     netip.Prefix
	Route       Route
	MTU         int
}
type Plan struct {
	Binding         Binding
	EffectiveExpiry time.Time
	Operations      []Operation
}

func sameNamespace(a, b Namespace) bool { return a == b && a.Isolated && a.Device != 0 && a.Inode != 0 }
func key(s string) bool {
	b, e := base64.StdEncoding.Strict().DecodeString(s)
	return e == nil && len(b) == 32 && base64.StdEncoding.EncodeToString(b) == s
}
func valid(b Binding, now time.Time) error {
	pool := netip.MustParsePrefix("100.64.0.0/10")
	for _, s := range []string{b.OwnIP, b.PeerIP} {
		a, e := netip.ParseAddr(s)
		if e != nil || !a.Is4() || a.String() != s || !pool.Contains(a) {
			return errors.New("exact assigned mesh IPv4 required")
		}
	}
	if b.OwnIP == b.PeerIP || !key(b.OwnKey) || !key(b.PeerKey) || b.OwnKey == b.PeerKey {
		return errors.New("distinct exact public identities required")
	}
	if !now.Before(b.SelfExpiry) || !now.Before(b.PeerExpiry) {
		return errors.New("original authority ended")
	}
	if len(b.Name) < 7 || len(b.Name) > 15 || !strings.HasPrefix(b.Name, "mno-") {
		return errors.New("dedicated interface name required")
	}
	for _, c := range b.Name {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789-", c) {
			return errors.New("invalid interface name")
		}
	}
	if len(b.Alias) != 36 || !strings.HasPrefix(b.Alias, "mno-") {
		return errors.New("fresh resource ownership alias required")
	}
	for _, c := range b.Alias[4:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return errors.New("invalid ownership alias")
		}
	}
	return nil
}

// Prepare refuses any preexisting target or mesh route/address. Default routes,
// foreign interfaces and policies also fail: this first executor is namespace-
// only, NOT a host-configurator. Empty lo-only inventory must be kernel checked.
func Prepare(b Binding, i Inventory, now time.Time) (Plan, error) {
	var p Plan
	if e := valid(b, now); e != nil {
		return p, e
	}
	if !sameNamespace(b.Namespace, i.Namespace) || i.ForeignPolicy || i.Forwarding {
		return p, ErrConflict
	}
	for _, l := range i.Links {
		if l.Name != "lo" || l.Kind != "loopback" {
			return p, ErrConflict
		}
	}
	pool := netip.MustParsePrefix("100.64.0.0/10")
	for _, r := range i.Routes {
		if !r.Destination.IsValid() || !r.Destination.Addr().IsLoopback() || r.Destination.Overlaps(pool) {
			return p, ErrConflict
		}
	}
	for _, a := range i.Addresses {
		if !a.Prefix.IsValid() || !a.Prefix.Addr().IsLoopback() {
			return p, ErrConflict
		}
	}
	p.Binding = b
	p.EffectiveExpiry = b.SelfExpiry
	if b.PeerExpiry.Before(p.EffectiveExpiry) {
		p.EffectiveExpiry = b.PeerExpiry
	}
	p.Operations = []Operation{{Kind: "create-nonpersistent-tun", Name: b.Name, Alias: b.Alias, MTU: MTU}, {Kind: "disable-owned-link-ipv6"}, {Kind: "address-add-exclusive", Address: netip.PrefixFrom(netip.MustParseAddr(b.OwnIP), 32)}, {Kind: "link-up-exact"}, {Kind: "route-add-exclusive", Route: Route{Destination: netip.PrefixFrom(netip.MustParseAddr(b.PeerIP), 32), Protocol: Protocol, Table: 254, Scope: "link", Type: "unicast", Metric: 42700, Priority: 42700}}}
	return p, nil
}

// Real execution binds this handle only after kernel ACK or readback matches the
// exclusive creation. Name alone is NEVER ownership; ifindex alone can be reused.
type Handle struct {
	Namespace Namespace
	Link      Link
	Address   Address
	Route     Route
}

func Attach(p Plan, index int) (Handle, error) {
	if len(p.Operations) != 5 || p.Operations[0].Kind != "create-nonpersistent-tun" || p.Operations[4].Kind != "route-add-exclusive" {
		return Handle{}, errors.New("invalid inert plan")
	}
	if index <= 0 {
		return Handle{}, errors.New("actual kernel ifindex required")
	}
	h := Handle{Namespace: p.Binding.Namespace, Link: Link{Index: index, Name: p.Binding.Name, Alias: p.Binding.Alias, Kind: "tun"}, Address: Address{Prefix: p.Operations[2].Address, LinkIndex: index}, Route: p.Operations[4].Route}
	h.Route.LinkIndex = index
	return h, nil
}

// Rollback returns only exact deletions visible now. A transport error must be
// followed by readback: absence is success, unchanged ownership stays pending,
// replacement means no mutation. It never assumes failed add had no effect.
func Rollback(h Handle, i Inventory) ([]Operation, error) {
	if !sameNamespace(h.Namespace, i.Namespace) {
		return nil, ErrConflict
	}
	var link *Link
	for n := range i.Links {
		l := &i.Links[n]
		if l.Index == h.Link.Index || l.Name == h.Link.Name || l.Alias == h.Link.Alias {
			if *l != h.Link {
				return nil, ErrConflict
			}
			if link != nil {
				return nil, ErrConflict
			}
			link = l
		}
	}
	if link == nil {
		return nil, nil
	}
	var ops []Operation
	for _, r := range i.Routes {
		if r.LinkIndex == h.Link.Index && r != h.Route && !kernelDerivedRoute(h, r) {
			return nil, ErrConflict
		}
		if r.Destination == h.Route.Destination {
			if r != h.Route {
				return nil, ErrConflict
			}
			ops = append(ops, Operation{Kind: "route-del-exact", Route: r})
		}
	}
	for _, a := range i.Addresses {
		if a.LinkIndex == h.Link.Index && a.Prefix != h.Address.Prefix && !kernelDerivedAddress(h, a) {
			return nil, fmt.Errorf("%w: unexpected address, including auto IPv6, requires pending cleanup", ErrConflict)
		}
		if a.LinkIndex == h.Link.Index && a.Prefix == h.Address.Prefix {
			ops = append(ops, Operation{Kind: "address-del-exact", Address: a.Prefix, Index: a.LinkIndex})
		}
	}
	ops = append(ops, Operation{Kind: "close-owned-tun-fd", Name: h.Link.Name, Alias: h.Link.Alias, Index: h.Link.Index})
	return ops, nil
}
func (p Plan) String() string {
	return fmt.Sprintf("inert Linux namespace plan: /32 self, /32 exact peer, effective expiry %s", p.EffectiveExpiry.UTC().Format(time.RFC3339Nano))
}

// KernelOrigin must come from audited netlink flags/protocol and exclusive
// namespace provenance, never caller claims in a future live executor. These
// items are recorded but NEVER explicitly deleted; original FD close removes
// kernel-derived state with the original link. Foreign/replaced state refuses.
func kernelDerivedAddress(h Handle, a Address) bool {
	return a.LinkIndex == h.Link.Index && a.KernelOrigin && a.Prefix.IsValid() && a.Prefix.Addr().Is6() && netip.MustParsePrefix("fe80::/10").Contains(a.Prefix.Addr()) && a.Prefix.Bits() == 64
}
func kernelDerivedRoute(h Handle, r Route) bool {
	if r.LinkIndex != h.Link.Index || !r.KernelOrigin || r.Protocol != 2 || r.Metric != 0 || r.Priority != 0 {
		return false
	}
	if r.Destination == h.Address.Prefix {
		return r.Table == 255 && r.Type == "local" && r.Scope == "host"
	}
	if r.Destination.IsValid() && r.Destination.Addr().Is6() {
		if r.Destination == netip.MustParsePrefix("ff00::/8") {
			return r.Table == 255 && r.Type == "multicast" && r.Scope == "link"
		}
		return r.Destination.Bits() == 64 && netip.MustParsePrefix("fe80::/10").Contains(r.Destination.Addr()) && r.Table == 254 && r.Type == "unicast" && r.Scope == "link"
	}
	return false
}
