//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/base64"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func rig() (Binding, Inventory, time.Time) {
	now := time.Now()
	ns := Namespace{Device: 1, Inode: 2, Isolated: true}
	return Binding{Namespace: ns, OwnIP: "100.64.0.1", PeerIP: "100.64.0.2", OwnKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)), PeerKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)), SelfExpiry: now.Add(time.Second), PeerExpiry: now.Add(2 * time.Second), Name: "mno-abc123", Alias: "mno-" + strings.Repeat("a", 32)}, Inventory{Namespace: ns, Links: []Link{{Index: 1, Name: "lo", Kind: "loopback"}}}, now
}
func TestPlanOnlyExactHostRoutesAndOriginalExpiry(t *testing.T) {
	b, i, now := rig()
	p, e := Prepare(b, i, now)
	if e != nil {
		t.Fatal(e)
	}
	if p.EffectiveExpiry != b.SelfExpiry || p.Operations[1].Address.Bits() != 32 || p.Operations[3].Route.Destination.String() != "100.64.0.2/32" {
		t.Fatal(p)
	}
	for _, op := range p.Operations {
		if strings.Contains(op.Kind, "replace") {
			t.Fatal("replacement planned")
		}
	}
	b.PeerExpiry = now.Add(500 * time.Millisecond)
	p, e = Prepare(b, i, now)
	if e != nil || p.EffectiveExpiry != b.PeerExpiry {
		t.Fatal("earlier peer deadline ignored")
	}
}
func TestPreparationRefusesForeignEffects(t *testing.T) {
	for _, name := range []string{"host", "namespace", "iface", "route", "address", "forwarding", "policy", "expiry", "identity", "name"} {
		t.Run(name, func(t *testing.T) {
			b, i, n := rig()
			switch name {
			case "host":
				i.Namespace.Isolated = false
			case "namespace":
				i.Namespace.Inode++
			case "iface":
				i.Links = append(i.Links, Link{Index: 4, Name: "eth0"})
			case "route":
				i.Routes = []Route{{Destination: netip.MustParsePrefix("0.0.0.0/0")}}
			case "address":
				i.Addresses = []Address{{Prefix: netip.MustParsePrefix("10.0.0.1/24")}}
			case "forwarding":
				i.Forwarding = true
			case "policy":
				i.ForeignPolicy = true
			case "expiry":
				b.SelfExpiry = n
			case "identity":
				b.PeerKey = b.OwnKey
			case "name":
				b.Name = "mnet0"
			}
			if _, e := Prepare(b, i, n); e == nil {
				t.Fatal("unsafe mutation plan accepted")
			}
		})
	}
}
func TestRollbackOnlyExactOwnedResource(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	i.Links = append(i.Links, h.Link)
	i.Addresses = []Address{h.Address}
	i.Routes = []Route{h.Route}
	ops, e := Rollback(h, i)
	if e != nil || len(ops) != 3 || ops[0].Kind != "route-del-exact" || ops[2].Kind != "close-owned-tun-fd" {
		t.Fatal(ops, e)
	}
	i.Links = i.Links[:1]
	i.Routes = nil
	i.Addresses = nil
	ops, e = Rollback(h, i)
	if e != nil || len(ops) != 0 {
		t.Fatal("absent original not idempotent")
	}
}
func TestRollbackRefusesIfindexNameAliasNamespaceAndRouteReplacement(t *testing.T) {
	for _, name := range []string{"ifindex", "name", "alias", "namespace", "route"} {
		t.Run(name, func(t *testing.T) {
			b, i, n := rig()
			p, _ := Prepare(b, i, n)
			h, _ := Attach(p, 7)
			l := h.Link
			i.Routes = []Route{h.Route}
			switch name {
			case "ifindex":
				l.Index = 8
			case "name":
				l.Name = "foreign"
			case "alias":
				l.Alias = "foreign"
			case "namespace":
				i.Namespace.Inode++
			case "route":
				i.Routes[0].Protocol = 99
			}
			i.Links = append(i.Links, l)
			ops, e := Rollback(h, i)
			if e == nil || len(ops) != 0 {
				t.Fatal("replacement targeted for delete")
			}
		})
	}
}
func TestMalformedPlanCannotAttachAndForeignAddressCannotBeDeleted(t *testing.T) {
	if _, e := Attach(Plan{}, 7); e == nil {
		t.Fatal("malformed plan accepted")
	}
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	i.Links = append(i.Links, h.Link)
	i.Addresses = []Address{{LinkIndex: 7, Prefix: netip.MustParsePrefix("100.64.0.9/32")}}
	if ops, e := Rollback(h, i); e == nil || len(ops) != 0 {
		t.Fatal("foreign mutation deleted")
	}
}
