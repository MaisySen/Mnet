//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

type faultKernel struct {
	*fakeKernel
	snap        int
	failSnap    int
	noHandle    bool
	indexZero   bool
	missingStep string
	cancelStep  string
	cancel      context.CancelFunc
	swapAt      int
}

func (k *faultKernel) Snapshot(ctx context.Context) (Inventory, error) {
	k.snap++
	if k.snap == k.failSnap {
		return Inventory{}, errors.New("snapshot unavailable")
	}
	i := cloneInventory(k.i)
	if k.snap == k.swapAt {
		i.Namespace.Inode++
	}
	return i, nil
}
func (k *faultKernel) Create(ctx context.Context, p Plan) (Handle, error) {
	h, e := k.fakeKernel.Create(ctx, p)
	if k.noHandle {
		return Handle{}, errors.New("ambiguous timeout without custody")
	}
	if k.indexZero {
		h.Link.Index = 0
	}
	return h, e
}
func (k *faultKernel) Apply(ctx context.Context, h Handle, o Operation) error {
	if k.cancelStep == o.Kind {
		k.cancel()
	}
	old := k.missingACK
	k.missingACK = o.Kind == k.missingStep
	e := k.fakeKernel.Apply(ctx, h, o)
	k.missingACK = old
	return e
}
func faultRig(t *testing.T) (*Transaction, *faultKernel) {
	tx, k, _ := transactionRig(t)
	f := &faultKernel{fakeKernel: k}
	tx.k = f
	return tx, f
}
func TestSnapshotFailureRetainsCustodyAtCreateApplyAndStop(t *testing.T) {
	for _, at := range []int{2, 3, 4, 5, 6} {
		tx, k := faultRig(t)
		k.failSnap = at
		if e := tx.Start(context.Background()); e == nil || !tx.PendingCleanup || tx.handle == nil {
			t.Fatal("snapshot error lost pending custody", at, e)
		}
		k.failSnap = 0
		if e := tx.Stop(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	tx, k := faultRig(t)
	if e := tx.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	k.failSnap = k.snap + 1
	before := len(k.calls)
	if e := tx.Stop(context.Background()); e == nil || !tx.PendingCleanup || len(k.calls) != before {
		t.Fatal("stop snapshot failure mutated or lost ownership")
	}
}
func TestAmbiguousCreateNeverAdoptsOrClaimsAbsence(t *testing.T) {
	for _, zero := range []bool{false, true} {
		tx, k := faultRig(t)
		k.noHandle = !zero
		k.indexZero = zero
		if e := tx.Start(context.Background()); e == nil || !tx.PendingCleanup || tx.Ready {
			t.Fatal("ambiguous creation accepted")
		}
		before := len(k.calls)
		if e := tx.Stop(context.Background()); e == nil || len(k.calls) != before {
			t.Fatal("ambiguous creation adopted by name")
		}
	}
}
func TestMissingEffectAtEachStepAndCancellation(t *testing.T) {
	for _, step := range []string{"disable-owned-link-ipv6", "address-add-exclusive", "link-up-exact", "route-add-exclusive"} {
		t.Run(step, func(t *testing.T) {
			tx, k := faultRig(t)
			k.missingStep = step
			if e := tx.Start(context.Background()); e == nil || tx.Ready || !tx.PendingCleanup {
				t.Fatal("ACK without effect accepted", e)
			}
			k.missingStep = ""
			if tx.Stop(context.Background()) != nil {
				t.Fatal("partial rollback failed")
			}
		})
	}
	tx, k := faultRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	k.cancel = cancel
	k.cancelStep = "address-add-exclusive"
	if e := tx.Start(ctx); !errors.Is(e, context.Canceled) || tx.Ready {
		t.Fatal("cancellation ignored", e)
	}
	for _, c := range k.calls {
		if c == "route-add-exclusive" {
			t.Fatal("later mutation after cancel")
		}
	}
}
func TestSnapshotNamespaceSwapFailsBeforeFurtherMutation(t *testing.T) {
	tx, k := faultRig(t)
	k.swapAt = 3
	if e := tx.Start(context.Background()); !errors.Is(e, ErrConflict) || tx.Ready {
		t.Fatal("namespace swap accepted", e)
	}
	if len(k.calls) != 2 {
		t.Fatal("mutated after swapped readback", k.calls)
	}
}
func TestDerivedRouteMetadataNotSwept(t *testing.T) {
	for _, field := range []string{"table", "proto", "metric", "type", "origin"} {
		tx, k := faultRig(t)
		if e := tx.Start(context.Background()); e != nil {
			t.Fatal(e)
		}
		r := Route{Destination: netip.MustParsePrefix("fe80::/64"), LinkIndex: 7, Protocol: 2, Table: 254, Scope: "link", Type: "unicast", KernelOrigin: true}
		switch field {
		case "table":
			r.Table = 123
		case "proto":
			r.Protocol = 99
		case "metric":
			r.Metric = 7
		case "type":
			r.Type = "blackhole"
		case "origin":
			r.KernelOrigin = false
		}
		k.i.Routes = append(k.i.Routes, r)
		before := len(k.calls)
		if e := tx.Stop(context.Background()); e == nil || !tx.PendingCleanup || len(k.calls) != before {
			t.Fatal("foreign derived metadata swept", field)
		}
	}
}
func TestStepEvidenceIncludesIntentACKAndReadback(t *testing.T) {
	tx, _, _ := transactionRig(t)
	if e := tx.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	for j, e := range tx.Evidence {
		if j%2 == 0 {
			if len(e.Step) < 6 || e.Step[len(e.Step)-7:] != "-intent" {
				t.Fatal("intent missing", e)
			}
		} else if !e.ACK || e.Error || e.Readback == "" {
			t.Fatal("ACK readback evidence missing", e)
		}
	}
}
