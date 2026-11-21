//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"testing"
)

type manyRecords struct{}

func (manyRecords) Request(context.Context, uint16, []byte, bool) ([][]byte, error) {
	return make([][]byte, 4097), nil
}
func TestInventoryRecordCapDirectly(t *testing.T) {
	_, e := ReadStableInventory(context.Background(), manyRecords{}, func() error { return nil })
	if e == nil || e.Error() != "inventory record cap" {
		t.Fatal("record cap not decisive", e)
	}
}
func TestAbsentDestinationChecksIndependentlyOfBaseline(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	if e := verifyAbsence(h, i, nil); e == nil || e.Error() != "baseline unavailable" {
		t.Fatal("nil baseline accepted")
	}
	for _, peer := range []bool{false, true} {
		bad := cloneInventory(i)
		address := h.Address
		address.LinkIndex = 8
		if peer {
			address.Prefix = h.Route.Destination
		}
		bad.Addresses = append(bad.Addresses, address)
		baseline := cloneInventory(bad)
		if e := verifyAbsence(h, bad, &baseline); e == nil || e.Error() != "owned destination address remains" {
			t.Fatal("direct address absence check not decisive", e)
		}
	}
	bad := cloneInventory(i)
	r := h.Route
	r.LinkIndex = 8
	bad.Routes = append(bad.Routes, r)
	baseline := cloneInventory(bad)
	if e := verifyAbsence(h, bad, &baseline); e == nil || e.Error() != "owned destination route remains" {
		t.Fatal("direct peer route check not decisive", e)
	}
}
func TestScopeMTUAndExactMutationGuards(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	h.Route.Scope = "global"
	if _, e := EncodeRoute(h.Route); e == nil {
		t.Fatal("wrong scope encoded")
	}
	if _, e := EncodeLink(7, b.Name, b.Alias, 1500, true); e == nil {
		t.Fatal("wrong mtu encoded")
	}
	h, _ = Attach(p, 7)
	if _, e := BuildExactMutation(h, Operation{Kind: "link-del"}); e == nil {
		t.Fatal("TUN link delete allowed")
	}
	w := &fakeWire{}
	if e := NewInMemoryNetlinkClient(w, 1).mutate(context.Background(), 17, nil); e == nil {
		t.Fatal("generic DELLINK allowed")
	}
	h.Route.Destination = h.Address.Prefix
	if _, e := BuildExactMutation(h, p.Operations[5]); e == nil {
		t.Fatal("wrong exact route encoded")
	}
}
func TestAliasMTUExplicitStageReadbackAndPendingRollback(t *testing.T) {
	tx, k, _ := transactionRig(t)
	k.missingACK = true
	if e := tx.Start(context.Background()); e == nil {
		t.Fatal("untagged link accepted")
	}
	if len(k.calls) != 2 || k.calls[1] != "set-alias-mtu-exact" {
		t.Fatal("alias/MTU not first ACK step", k.calls)
	}
	k.missingACK = false
	if e := tx.Stop(context.Background()); e != nil {
		t.Fatal("unconfigured link custody rollback failed", e)
	}
	for _, f := range []string{"mtu", "alias"} {
		tx, k := faultRig(t)
		k.missingStep = "set-alias-mtu-exact"
		_ = tx.Start(context.Background())
		if tx.Ready {
			t.Fatal(f, "became ready")
		}
	}
}

type closeWire struct {
	wireFunc
	closes int
}

func (w *closeWire) Close() error { w.closes++; return nil }
func TestSuccessExchangeClosesTransport(t *testing.T) {
	w := &closeWire{wireFunc: func(r []byte) ([]Datagram, error) { return []Datagram{{Bytes: ackFrame(r, 0)}}, nil }}
	_, e := NewInMemoryNetlinkClient(w, 1).requestFixture(context.Background(), 16, nil, false)
	if e != nil || w.closes != 1 {
		t.Fatal("successful socket not closed", e, w.closes)
	}
}

type changingDump struct{ calls int }

func (d *changingDump) Request(_ context.Context, k uint16, _ []byte, _ bool) ([][]byte, error) {
	d.calls++
	if k == 34 {
		b := make([]byte, 12)
		b[0] = byte(d.calls)
		return [][]byte{b}, nil
	}
	return nil, nil
}
func TestTwoEqualInventoriesRequired(t *testing.T) {
	d := &changingDump{}
	_, e := ReadStableInventory(context.Background(), d, func() error { return nil })
	if e == nil || d.calls != 16 {
		t.Fatal("different complete sets accepted", e, d.calls)
	}
}
func TestBaselineMapsMultisetAndCopyIndependent(t *testing.T) {
	_, i, _ := rig()
	i.LinkUp = map[int]bool{1: true}
	copy := cloneInventory(i)
	copy.LinkUp[1] = false
	if !i.LinkUp[1] || equalInventoryBaseline(i, copy) {
		t.Fatal("map clone/comparison lost")
	}
	a := i
	a.Links = append(a.Links, Link{Index: 9})
	b := i
	b.Links = append([]Link{i.Links[0]}, i.Links[0])
	if equalInventoryBaseline(a, b) {
		t.Fatal("duplicate multiset accepted")
	}
}
func TestExactTypedOperationsEncodePlannedIdentity(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	for _, op := range p.Operations[1:] {
		if op.Kind == "disable-owned-link-ipv6" {
			continue
		}
		m, e := BuildExactMutation(h, op)
		if e != nil {
			t.Fatal(op.Kind, e)
		}
		w := &fakeWire{}
		if e = NewInMemoryNetlinkClient(w, 1).ApplyExact(context.Background(), m); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := NewInMemoryNetlinkClient(&fakeWire{}, 1).Request(context.Background(), 16, nil, false); e == nil {
		t.Fatal("public raw mutation still possible")
	}
}
