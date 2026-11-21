//go:build mnet_enrollment_development

package osownership

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"sort"
)

// CapturedBaseline is a SOURCE candidate only. Construction never authorizes
// mutation. Initial down/unaddressed links are immutable resources, not owned
// links and not a name allowlist. Raw attrs remain part of every comparison.
type CapturedBaseline struct {
	ns     Namespace
	raw    RawInventory
	sys    map[string]string
	digest string
}
type SysctlSnapshot map[string]string

func validateForwarding(raw RawInventory, s SysctlSnapshot) error {
	expected := map[string]bool{"/proc/sys/net/ipv4/ip_forward": true}
	names := []string{"all", "default"}
	for _, l := range raw.Links {
		if !safeInterfaceName(l.Name) {
			return ErrConflict
		}
		expected["/proc/sys/net/ipv4/conf/"+l.Name+"/forwarding"] = true
		yes, e := LinkIPv6Applicable(l)
		if e != nil {
			return e
		}
		if e = validateIPv6Directory(l.Name, yes, s); e != nil {
			return fmt.Errorf("link %s: %w", l.Name, e)
		}
	}
	for _, name := range names {
		expected["/proc/sys/net/ipv4/conf/"+name+"/forwarding"] = true
		if e := validateIPv6Directory(name, true, s); e != nil {
			return e
		}
	}
	for k := range expected {
		if s[k] != "0" {
			return fmt.Errorf("missing/enabled forwarding %s", k)
		}
	}
	return nil
}
func safeInterfaceName(s string) bool {
	if len(s) == 0 || len(s) > 15 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return s != "." && s != ".."
}
func NewBaselineCandidate(parent, ns Namespace, raw RawInventory, s SysctlSnapshot) (*CapturedBaseline, error) {
	if parent.Device == 0 || parent.Inode == 0 || parent.Device == ns.Device && parent.Inode == ns.Inode || !sameNamespace(ns, ns) || len(raw.Links) == 0 || len(raw.Addresses) != 0 || len(raw.Routes) != 0 {
		return nil, errors.New("baseline must be distinct, all down, unaddressed, unrouted")
	}
	if e := ValidateDefaultRules(raw.Rules); e != nil {
		return nil, e
	}
	if e := validateForwarding(raw, s); e != nil {
		return nil, e
	}
	seen := map[int]bool{}
	names := map[string]bool{}
	for _, l := range raw.Links {
		if l.Flags&1 != 0 || l.Alias != "" || l.MTU == 0 || seen[l.Index] || names[l.Name] || !safeInterfaceName(l.Name) {
			return nil, ErrConflict
		}
		seen[l.Index] = true
		names[l.Name] = true
		if underlay, e := attrUint(l.Raw, 5, false); e != nil || underlay != 0 {
			return nil, errors.New("baseline has underlay linkage")
		}
		kind, e := linkKind(l)
		if e != nil {
			return nil, e
		}
		switch kind {
		case "veth", "vxlan", "ipvlan", "macvlan", "macvtap", "geneve":
			return nil, errors.New("cross-namespace-capable baseline link refused")
		}
		if _, yes := l.Raw[10]; yes {
			return nil, errors.New("baseline has master")
		}
		if _, yes := l.Raw[37]; yes {
			return nil, errors.New("baseline has cross-namespace link")
		}
	}
	b, e := json.Marshal(raw)
	if e != nil {
		return nil, e
	}
	// Structural clone also retains private original ordered dump evidence,
	// which JSON intentionally does not expose. Hash format is unchanged.
	frozen := cloneRawExact(raw)
	sys := map[string]string{}
	for k, v := range s {
		sys[k] = v
	}
	hash := sha256.Sum256(append(b, mustJSON(sys)...))
	return &CapturedBaseline{ns: ns, raw: frozen, sys: sys, digest: hex.EncodeToString(hash[:])}, nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// Compose refuses changes to any raw baseline attribute, even an unmodeled
// attribute. Origin is derived narrowly from ACK witness+exact local-route
// shape, never solely RTPROT_KERNEL. IPv6 derived effects block this candidate.
func (b *CapturedBaseline) Compose(raw RawInventory, s SysctlSnapshot, h *Handle, addressACK bool) (Inventory, error) {
	return b.composeWithOwnedAF(raw, s, h, addressACK, nil)
}

// Private path admits26 only when a current ledger/pass proof is supplied.
func (b *CapturedBaseline) composeWithOwnedAF(raw RawInventory, s SysctlSnapshot, h *Handle, addressACK bool, proof *ownedAFComposeProof) (Inventory, error) {
	var out Inventory
	if b == nil {
		return out, ErrConflict
	}
	if e := ValidateDefaultRules(raw.Rules); e != nil {
		return out, e
	}
	if !reflect.DeepEqual(raw.Rules, b.raw.Rules) {
		return out, errors.New("raw rules baseline changed")
	}
	if e := validateForwarding(raw, s); e != nil {
		return out, e
	}
	out = Inventory{Namespace: b.ns, LinkUp: map[int]bool{}, IPv6Disabled: map[int]bool{}}
	remaining := append([]LinkRecord(nil), raw.Links...)
	for _, old := range b.raw.Links {
		found := -1
		for j, l := range remaining {
			if l.Index == old.Index {
				if !reflect.DeepEqual(l, old) {
					return out, errors.New("immutable raw baseline link changed")
				}
				found = j
				break
			}
		}
		if found < 0 {
			return out, errors.New("baseline link disappeared")
		}
		remaining = append(remaining[:found], remaining[found+1:]...)
		kind := "baseline-immutable"
		if old.Hardware == 772 {
			kind = "loopback"
		}
		out.Links = append(out.Links, Link{Index: old.Index, Name: old.Name, Alias: old.Alias, Kind: kind, MTU: old.MTU})
		out.LinkUp[old.Index] = false
		out.IPv6Disabled[old.Index] = s["/proc/sys/net/ipv6/conf/"+old.Name+"/disable_ipv6"] == "1"
	}
	for k, v := range b.sys {
		if s[k] != v {
			return out, fmt.Errorf("baseline sysctl changed %s", k)
		}
	}
	if h == nil && len(remaining) != 0 || len(remaining) > 1 {
		return out, errors.New("unexpected link")
	}
	if len(remaining) == 1 {
		l := remaining[0]
		for k := range l.Raw {
			if k == 26 && proof != nil && proof.matches(b, l, s, h) {
				continue
			}
			if k != 3 && k != 4 && k != 18 && k != 20 {
				return out, fmt.Errorf("unreviewed owned link attr %d", k)
			}
		}
		kind, e := linkKind(l)
		if e != nil {
			return out, e
		}
		actual := Link{Index: l.Index, Name: l.Name, Alias: l.Alias, Kind: kind, MTU: l.MTU}
		if !handleLinkMatches(*h, actual) {
			return out, ErrConflict
		}
		out.Links = append(out.Links, actual)
		out.LinkUp[l.Index] = l.Flags&1 != 0
		out.IPv6Disabled[l.Index] = s["/proc/sys/net/ipv6/conf/"+l.Name+"/disable_ipv6"] == "1"
	}
	for _, a := range raw.Addresses {
		if e := validateOwnedAddress(h, a); e != nil {
			return out, e
		}
		out.Addresses = append(out.Addresses, h.Address)
	}
	for _, r := range raw.Routes {
		if h == nil {
			return out, ErrConflict
		}
		route, e := composeRoute(*h, r, addressACK)
		if e != nil {
			return out, e
		}
		out.Routes = append(out.Routes, route)
	}
	out.baselineLinks = append([]Link(nil), out.Links[:len(b.raw.Links)]...)
	out.baselineFingerprint = b.digest
	sort.Slice(out.Links, func(i, j int) bool { return out.Links[i].Index < out.Links[j].Index })
	return out, nil
}
func linkKind(l LinkRecord) (string, error) {
	if l.Hardware == 772 {
		return "loopback", nil
	}
	a, e := decodeAttributes(l.Raw[18])
	if e != nil {
		return "", e
	}
	return attrText(a, 1, true)
}
func composeRoute(h Handle, r RouteRecord, addressACK bool) (Route, error) {
	if r.Table != 0 && uint32(r.Table) != r.TableID {
		return Route{}, ErrConflict
	}
	if r.Family != 2 || r.DestBits != 32 || r.SourceBits != 0 || r.TOS != 0 || r.Flags != 0 || r.Source.IsValid() || r.Gateway.IsValid() || r.IIF != 0 || int(r.OIF) != h.Link.Index {
		return Route{}, ErrConflict
	}
	for k := range r.Raw {
		if k != 1 && k != 4 && k != 6 && k != 7 && k != 15 {
			return Route{}, fmt.Errorf("unreviewed route attr %d", k)
		}
	}
	if r.Dst == h.Route.Destination.Addr() && r.Protocol == 242 && r.TableID == 254 && r.Scope == 253 && r.Type == 1 && r.Priority == h.Route.Priority && !r.PreferredSource.IsValid() {
		return h.Route, nil
	}
	if addressACK && r.Dst == h.Address.Prefix.Addr() && r.Protocol == 2 && r.TableID == 255 && r.Scope == 254 && r.Type == 2 && r.Priority == 0 && r.PreferredSource == h.Address.Prefix.Addr() {
		return Route{Destination: netip.PrefixFrom(r.Dst, 32), LinkIndex: h.Link.Index, Protocol: 2, Table: 255, Scope: "host", Type: "local", KernelOrigin: true}, nil
	}
	return Route{}, ErrConflict
}

func validateOwnedAddress(h *Handle, a AddressRecord) error {
	if h == nil || a.Index != h.Link.Index || a.Family != 2 || a.Prefix != 32 || a.Local != h.Address.Prefix.Addr() || a.Address != a.Local || a.Scope != 0 {
		return ErrConflict
	}
	flags, e := attrUint(a.Raw, 8, true)
	if e != nil || flags != 0x280 || a.Flags != 0x80 {
		return errors.New("owned address lacks no-prefix-route")
	}
	for k := range a.Raw {
		if k != 1 && k != 2 && k != 8 {
			return fmt.Errorf("unreviewed address attr %d", k)
		}
	}
	return nil
}
