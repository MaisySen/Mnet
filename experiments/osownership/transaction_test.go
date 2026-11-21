//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"
)

type fakeClock struct{ s Stamp }

func (c *fakeClock) Now() Stamp { return c.s }

type fakeKernel struct {
	i              Inventory
	h              Handle
	calls          []string
	fault          string
	successOnError bool
	clock          *fakeClock
	advance        string
	ipv6Auto       bool
	missingACK     bool
}

func (k *fakeKernel) Snapshot(context.Context) (Inventory, error) { return k.i, nil }
func (k *fakeKernel) Create(_ context.Context, p Plan) (Handle, error) {
	k.calls = append(k.calls, "create")
	h, e := Attach(p, 7)
	if e != nil {
		return h, e
	}
	k.h = h
	if k.fault != "create" || k.successOnError {
		k.i.Links = append(k.i.Links, h.Link)
	}
	if k.fault == "create" {
		return h, errors.New("create transport timeout")
	}
	return h, nil
}
func (k *fakeKernel) Apply(_ context.Context, h Handle, o Operation) error {
	k.calls = append(k.calls, o.Kind)
	bad := k.fault == o.Kind
	if k.advance == o.Kind {
		k.clock.s.Monotonic += 2 * time.Second
		k.clock.s.Wall = k.clock.s.Wall.Add(-time.Hour)
	}
	if !bad || k.successOnError {
		if !k.missingACK {
			switch o.Kind {
			case "disable-owned-link-ipv6":
				k.i.IPv6Disabled[h.Link.Index] = true
			case "address-add-exclusive":
				k.i.Addresses = append(k.i.Addresses, h.Address)
				k.i.Routes = append(k.i.Routes, Route{Destination: h.Address.Prefix, LinkIndex: 7, Protocol: 2, Table: 255, Type: "local", Scope: "host", KernelOrigin: true})
			case "link-up-exact":
				k.i.LinkUp[7] = true
				if k.ipv6Auto {
					k.i.Addresses = append(k.i.Addresses, Address{Prefix: netip.MustParsePrefix("fe80::1/64"), LinkIndex: 7, KernelOrigin: true})
					k.i.Routes = append(k.i.Routes, Route{Destination: netip.MustParsePrefix("fe80::/64"), LinkIndex: 7, Protocol: 2, Table: 254, Type: "unicast", Scope: "link", KernelOrigin: true})
				}
			case "route-add-exclusive":
				k.i.Routes = append(k.i.Routes, h.Route)
			case "route-del-exact":
				var next []Route
				for _, r := range k.i.Routes {
					if r != o.Route {
						next = append(next, r)
					}
				}
				k.i.Routes = next
			case "address-del-exact":
				var next []Address
				for _, a := range k.i.Addresses {
					if a.Prefix != o.Address || a.LinkIndex != o.Index {
						next = append(next, a)
					}
				}
				k.i.Addresses = next
			}
		}
	}
	if bad {
		return errors.New("transport timeout; success unknown")
	}
	return nil
}
func (k *fakeKernel) CloseOriginalFD(h Handle) error {
	k.calls = append(k.calls, "close-fd")
	delete(k.i.IPv6Disabled, h.Link.Index)
	delete(k.i.LinkUp, h.Link.Index)
	var links []Link
	for _, l := range k.i.Links {
		if l != h.Link {
			links = append(links, l)
		}
	}
	k.i.Links = links
	var rs []Route
	for _, r := range k.i.Routes {
		if r.LinkIndex != h.Link.Index {
			rs = append(rs, r)
		}
	}
	k.i.Routes = rs
	var as []Address
	for _, a := range k.i.Addresses {
		if a.LinkIndex != h.Link.Index {
			as = append(as, a)
		}
	}
	k.i.Addresses = as
	return nil
}
func transactionRig(t *testing.T) (*Transaction, *fakeKernel, *fakeClock) {
	t.Helper()
	b, i, now := rig()
	i.IPv6Disabled = map[int]bool{}
	i.LinkUp = map[int]bool{}
	c := &fakeClock{Stamp{Wall: now, Monotonic: time.Second}}
	k := &fakeKernel{i: i, clock: c}
	tx, e := NewTransaction(k, c, b)
	if e != nil {
		t.Fatal(e)
	}
	return tx, k, c
}
func TestACKReadbackAndExactStop(t *testing.T) {
	tx, k, _ := transactionRig(t)
	if e := tx.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	if !tx.Ready || !tx.PendingCleanup || len(tx.Evidence) != 10 {
		t.Fatal("step evidence missing", tx.Evidence)
	}
	if e := tx.Stop(context.Background()); e != nil || tx.PendingCleanup {
		t.Fatal(e)
	}
	if len(k.i.Links) != 1 || len(k.i.Routes) != 0 || len(k.i.Addresses) != 0 {
		t.Fatal("fake kernel residue")
	}
}
func TestEachPartialTimeoutAfterSuccessReadbackRollback(t *testing.T) {
	for _, step := range []string{"create", "disable-owned-link-ipv6", "address-add-exclusive", "link-up-exact", "route-add-exclusive"} {
		t.Run(step, func(t *testing.T) {
			tx, k, _ := transactionRig(t)
			k.fault = step
			k.successOnError = true
			if tx.Start(context.Background()) == nil || tx.Ready {
				t.Fatal("timeout became success")
			}
			if tx.handle == nil || !tx.PendingCleanup {
				t.Fatal("partial custody lost")
			}
			k.fault = ""
			if e := tx.Stop(context.Background()); e != nil || tx.PendingCleanup {
				t.Fatal("rollback failed", e)
			}
			if len(k.i.Links) != 1 || len(k.i.Routes) != 0 {
				t.Fatal("residue")
			}
		})
	}
}
func TestACKWithoutEffectAndMonotonicExpiryAbort(t *testing.T) {
	tx, k, _ := transactionRig(t)
	k.missingACK = true
	if tx.Start(context.Background()) == nil || tx.Ready {
		t.Fatal("ACK without readback accepted")
	}
	tx, k, _ = transactionRig(t)
	k.advance = "address-add-exclusive"
	if tx.Start(context.Background()) == nil || !tx.Expired() {
		t.Fatal("wall rewind revived monotonic authority")
	}
	for _, call := range k.calls {
		if call == "route-add-exclusive" {
			t.Fatal("route added after effective deadline")
		}
	}
	if tx.Stop(context.Background()) != nil {
		t.Fatal("expired cleanup blocked")
	}
}
func TestUnexpectedKernelIPv6StopsButExactDerivedCleanupWorks(t *testing.T) {
	tx, k, _ := transactionRig(t)
	k.ipv6Auto = true
	if tx.Start(context.Background()) == nil || tx.Ready {
		t.Fatal("IPv6 up side effect accepted despite disable ACK")
	}
	if e := tx.Stop(context.Background()); e != nil {
		t.Fatal("kernel linklocal made own FD cleanup impossible", e)
	}
	if len(k.i.Addresses) != 0 || len(k.i.Routes) != 0 {
		t.Fatal("derived IPv6 state not removed with owned link")
	}
	for _, c := range k.calls {
		if c == "ipv6-delete" {
			t.Fatal("broad IPv6 cleanup")
		}
	}
}
func TestReplacementAndRouteMetricPreventCleanup(t *testing.T) {
	for _, field := range []string{"ifindex", "alias", "metric", "priority", "type", "foreign-ipv6"} {
		t.Run(field, func(t *testing.T) {
			tx, k, _ := transactionRig(t)
			if e := tx.Start(context.Background()); e != nil {
				t.Fatal(e)
			}
			switch field {
			case "ifindex":
				k.i.Links[1].Index++
			case "alias":
				k.i.Links[1].Alias = "foreign"
			case "metric":
				k.i.Routes[len(k.i.Routes)-1].Metric++
			case "priority":
				k.i.Routes[len(k.i.Routes)-1].Priority++
			case "type":
				k.i.Routes[len(k.i.Routes)-1].Type = "blackhole"
			case "foreign-ipv6":
				k.i.Addresses = append(k.i.Addresses, Address{Prefix: netip.MustParsePrefix("fe80::1/64"), LinkIndex: 7, KernelOrigin: false})
			}
			before := len(k.calls)
			if e := tx.Stop(context.Background()); e == nil || !tx.PendingCleanup {
				t.Fatal("replacement swept away")
			}
			if len(k.calls) != before {
				t.Fatal("mutation despite conflict")
			}
		})
	}
}
func TestCleanupTimeoutReadbackAbsenceVersusStillPending(t *testing.T) {
	for _, success := range []bool{true, false} {
		tx, k, _ := transactionRig(t)
		if e := tx.Start(context.Background()); e != nil {
			t.Fatal(e)
		}
		k.fault = "route-del-exact"
		k.successOnError = success
		e := tx.Stop(context.Background())
		if success && (e != nil || tx.PendingCleanup) {
			t.Fatal("confirmed absence not handled")
		}
		if !success && (e == nil || !tx.PendingCleanup) {
			t.Fatal("unconfirmed delete lost ownership")
		}
	}
}
func TestOwnerRunJoinsFakeResourcesAtDeadlineWithoutCallerStop(t *testing.T) {
	b, i, now := rig()
	b.SelfExpiry = now.Add(5 * time.Millisecond)
	i.IPv6Disabled = map[int]bool{}
	i.LinkUp = map[int]bool{}
	c := &fakeClock{Stamp{Wall: now, Monotonic: time.Second}}
	k := &fakeKernel{i: i, clock: c}
	tx, e := NewTransaction(k, c, b)
	if e != nil {
		t.Fatal(e)
	}
	if tx.Run(context.Background()) == nil || tx.PendingCleanup || tx.Ready {
		t.Fatal("owner did not retire and join at deadline")
	}
	if len(k.i.Links) != 1 {
		t.Fatal("fake original interface survived")
	}
}
