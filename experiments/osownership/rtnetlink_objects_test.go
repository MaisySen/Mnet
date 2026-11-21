//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestRouteAddressLinkFixturesAndExactDeletePayload(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	rb, e := EncodeRoute(h.Route)
	if e != nil {
		t.Fatal(e)
	}
	r, e := DecodeRoute(rb)
	if e != nil || r.Dst != netip.MustParseAddr(b.PeerIP) || r.OIF != 7 || r.Priority != 42700 || r.Type != 1 || r.TableID != 254 {
		t.Fatal(r, e)
	}
	ab, e := EncodeAddress(7, netip.MustParseAddr(b.OwnIP))
	if e != nil {
		t.Fatal(e)
	}
	a, e := DecodeAddress(ab)
	if e != nil || a.Local.String() != b.OwnIP || a.Prefix != 32 {
		t.Fatal(a, e)
	}
	if v, e := attrUint(a.Raw, 8, true); e != nil || v != 0x200 {
		t.Fatal("NOPREFIXROUTE missing")
	}
	lb, e := EncodeLink(7, b.Name, b.Alias, 1280, true)
	if e != nil {
		t.Fatal(e)
	}
	l, e := DecodeLink(lb)
	if e != nil || l.Index != 7 || l.Name != b.Name || l.Alias != b.Alias || l.MTU != 1280 || l.Flags&1 == 0 {
		t.Fatal(l, e)
	}
	w := &fakeWire{}
	if e := NewInMemoryNetlinkClient(w, 1).mutate(context.Background(), 25, rb); e != nil {
		t.Fatal(e)
	}
	if binary.NativeEndian.Uint16(w.request[6:]) != 5 {
		t.Fatal("delete had create/replace flags")
	}
	h.Route.Metric++
	if _, e := EncodeRoute(h.Route); e == nil {
		t.Fatal("inconsistent metric priority encoded")
	}
}
func TestObjectMalformedDuplicateUnknownAndForeignFixtures(t *testing.T) {
	for _, raw := range [][]byte{nil, {1, 0, 0, 0}, {4, 0, 1, 0, 4, 0, 1, 0}, {5, 0, 1, 0, 0, 1, 0, 0}, {4, 0, 1, 128}} {
		if _, e := decodeAttributes(raw); e == nil && len(raw) > 0 {
			t.Fatal("bad attrs accepted", raw)
		}
	}
	raw := attr(999, []byte{1, 2})
	a, e := decodeAttributes(raw)
	if e != nil || len(a[999]) != 2 {
		t.Fatal("unknown data lost")
	}
	if _, e := DecodeLink(make([]byte, 15)); e == nil {
		t.Fatal("short link")
	}
	if _, e := DecodeAddress(make([]byte, 8)); e == nil {
		t.Fatal("bad address")
	}
	if _, e := DecodeRoute(make([]byte, 12)); e == nil {
		t.Fatal("bad route")
	}
	r := make([]byte, 12)
	r[0] = 2
	r[1] = 32
	if _, e := DecodeRoute(r); e == nil {
		t.Fatal("missing nondefault destination")
	}
	r[1] = 0
	rr, e := DecodeRoute(r)
	if e != nil || rr.Dst != netip.IPv4Unspecified() {
		t.Fatal(rr, e)
	}
}

type fixtureDump struct {
	calls     int
	failFirst bool
	failAll   bool
}

func (f *fixtureDump) Request(_ context.Context, _ uint16, _ []byte, _ bool) ([][]byte, error) {
	f.calls++
	if f.failAll || f.failFirst && f.calls == 2 {
		return nil, errors.New("interrupted dump")
	}
	return nil, nil
}
func TestWholeInventoryRetryBoundAndNamespaceCheck(t *testing.T) {
	d := &fixtureDump{failFirst: true}
	checks := 0
	if _, e := ReadStableInventory(context.Background(), d, func() error { checks++; return nil }); e != nil || d.calls != 10 || checks == 0 {
		t.Fatal("whole inventory retry incorrect", d.calls, e)
	}
	d = &fixtureDump{failAll: true}
	if _, e := ReadStableInventory(context.Background(), d, func() error { return nil }); e == nil || d.calls != 2 {
		t.Fatal("unbounded or accepted interrupted dumps")
	}
	d = &fixtureDump{}
	if _, e := ReadStableInventory(context.Background(), d, func() error { return ErrConflict }); e == nil || d.calls != 0 {
		t.Fatal("foreign namespace dumped")
	}
}
func TestFullAbsenceRejectsOrphansAndChangedBaseline(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	baseline := cloneInventory(i)
	for _, which := range []string{"route", "address", "foreign-link"} {
		bad := cloneInventory(i)
		switch which {
		case "route":
			bad.Routes = []Route{h.Route}
			bad.Routes[0].LinkIndex = 8
		case "address":
			bad.Addresses = []Address{h.Address}
			bad.Addresses[0].LinkIndex = 8
		case "foreign-link":
			bad.Links = append(bad.Links, Link{Index: 8, Name: "foreign"})
		}
		if e := verifyAbsence(h, bad, &baseline); e == nil {
			t.Fatal("orphan/baseline change accepted", which)
		}
	}
}
func TestTransactionFDClosedOnce(t *testing.T) {
	tx, k, _ := transactionRig(t)
	if e := tx.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := tx.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := tx.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, c := range k.calls {
		if strings.Contains(c, "close-fd") {
			count++
		}
	}
	if count != 1 {
		t.Fatal("original FD closed more than once", count)
	}
}
func TestDummyEncoderAndNestedLinkInfoFixture(t *testing.T) {
	b, i, n := rig()
	_ = i
	_ = n
	raw, e := EncodeDummy(b.Name, b.Alias)
	if e != nil {
		t.Fatal(e)
	}
	binary.NativeEndian.PutUint32(raw[4:], 7)
	r, e := DecodeLink(raw)
	if e != nil || r.Name != b.Name {
		t.Fatal(r, e)
	}
	a, e := decodeAttributes(r.Raw[18])
	if e != nil {
		t.Fatal(e)
	}
	kind, e := attrText(a, 1, true)
	if e != nil || kind != "dummy" {
		t.Fatal(kind, e)
	}
	w := &fakeWire{}
	if e = NewInMemoryNetlinkClient(w, 1).createDummy(context.Background(), raw); e != nil {
		t.Fatal(e)
	}
	if binary.NativeEndian.Uint16(w.request[6:]) != 1|4|0x200|0x400 {
		t.Fatal("dummy not exclusive")
	}
}
