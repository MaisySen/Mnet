//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"testing"
)

// Emitter-shaped standalone fixture, not EncodeAddress (request encoder).
// Independent cstamp/tstamp include zero/wrap forms; not a live kernel oracle.
func addressEmitterFixture(h *Handle, created, updated uint32) []byte {
	b := make([]byte, 8)
	b[0], b[1], b[2] = 2, 32, 0x80
	binary.NativeEndian.PutUint32(b[4:], uint32(h.Link.Index))
	for _, a := range [][]byte{
		attr(1, ipBytes(h.Address.Prefix.Addr())), attr(2, ipBytes(h.Address.Prefix.Addr())),
		attr(3, append([]byte(h.Link.Name), 0)), attr(8, u32(0x280)),
		attr(6, append(append(append(u32(0xffffffff), u32(0xffffffff)...), u32(created)...), u32(updated)...)),
	} {
		b = append(b, a...)
	}
	return b
}
func TestOwnedAddressDumpCandidateEmitterAndExactCache(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	for _, timestamps := range [][2]uint32{{0, 123}, {99, 3}, {0xffffffff, 0}, {45, 45}} {
		body := addressEmitterFixture(&h, timestamps[0], timestamps[1])
		c, e := captureOwnedAddressDumpCandidate(&h, body)
		if e != nil || !c.matches(&h, body) {
			t.Fatal("emitter candidate", e)
		}
		a, _ := DecodeAddress(body)
		if a.Flags != 0x80 || binary.NativeEndian.Uint32(a.Raw[8]) != 0x280 {
			t.Fatal("header/full flags split")
		}
		for word := 0; word < 4; word++ {
			changed := addressEmitterFixture(&h, timestamps[0], timestamps[1])
			off := len(changed) - 16 + word*4
			changed[off] ^= 1
			if c.matches(&h, changed) {
				t.Fatal("cache drift accepted", word)
			}
		}
		body[len(body)-1] ^= 1
		if c.matches(&h, body) {
			t.Fatal("candidate aliased input")
		}
	}
}
func TestOwnedAddressDumpCandidateMissingExtraAndMalformed(t *testing.T) {
	b, i, n := rig()
	p, _ := Prepare(b, i, n)
	h, _ := Attach(p, 7)
	body := addressEmitterFixture(&h, 0, 7)
	a, _ := DecodeAddress(body)
	rebuild := func(attrs map[uint16][]byte) []byte {
		out := append([]byte(nil), body[:8]...)
		for _, k := range []uint16{1, 2, 3, 8, 6, 4, 9, 11, 999} {
			if v, ok := attrs[k]; ok {
				out = append(out, attr(k, v)...)
			}
		}
		return out
	}
	for _, k := range []uint16{1, 2, 3, 8, 6} {
		attrs := map[uint16][]byte{}
		for key, v := range a.Raw {
			if key != k {
				attrs[key] = v
			}
		}
		if _, e := captureOwnedAddressDumpCandidate(&h, rebuild(attrs)); e == nil {
			t.Fatal("missing accepted", k)
		}
	}
	for _, k := range []uint16{4, 9, 11, 999} {
		attrs := map[uint16][]byte{}
		for key, v := range a.Raw {
			attrs[key] = v
		}
		attrs[k] = u32(0)
		if _, e := captureOwnedAddressDumpCandidate(&h, rebuild(attrs)); e == nil {
			t.Fatal("extra accepted", k)
		}
	}
	mutations := []func([]byte) []byte{
		func(x []byte) []byte { x[2] = 0; return x },
		func(x []byte) []byte { x[3] = 254; return x },
		func(x []byte) []byte { x[1] = 31; return x },
		func(x []byte) []byte { return append(x, attr(6, make([]byte, 16))...) },
		func(x []byte) []byte { return append(x, attr(8|0x8000, u32(0x280))...) },
	}
	for j, f := range mutations {
		if _, e := captureOwnedAddressDumpCandidate(&h, f(append([]byte(nil), body...))); e == nil {
			t.Fatal("bad dump accepted", j)
		}
	}
	for _, k := range []uint16{1, 2, 3, 8, 6} {
		attrs := map[uint16][]byte{}
		for key, v := range a.Raw {
			attrs[key] = v
		}
		attrs[k] = []byte{0}
		if _, e := captureOwnedAddressDumpCandidate(&h, rebuild(attrs)); e == nil {
			t.Fatal("wrong shape accepted", k)
		}
	}
	attrs := map[uint16][]byte{}
	for key, v := range a.Raw {
		attrs[key] = v
	}
	attrs[3] = append([]byte("foreign"), 0)
	if _, e := captureOwnedAddressDumpCandidate(&h, rebuild(attrs)); e == nil {
		t.Fatal("foreign label accepted")
	}
}
func TestAddressTransitionScaffoldCannotSendOrAdvance(t *testing.T) {
	for _, ctx := range []context.Context{nil, context.Background()} {
		if r, e := refuseAddressDownSend(ctx, &postDisableResult{}, &ownedAddressDumpCandidate{}); r != nil || e == nil {
			t.Fatal("send released")
		}
		for _, r := range []*postAddressDownResult{nil, {}, {disabledAncestry: &postDisableResult{}, lifetime: &disableResidentLifetime{}, address: &ownedAddressDumpCandidate{}}} {
			if r.current() || r.admitStep4(ctx) == nil {
				t.Fatal("step4 released")
			}
		}
	}
}
