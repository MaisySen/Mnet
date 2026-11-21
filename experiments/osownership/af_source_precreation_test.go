//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"sort"
	"testing"
)

func afLinkBodyTest(l LinkRecord) []byte {
	b := make([]byte, 16)
	binary.NativeEndian.PutUint16(b[2:], l.Hardware)
	binary.NativeEndian.PutUint32(b[4:], uint32(l.Index))
	binary.NativeEndian.PutUint32(b[8:], l.Flags)
	var keys []int
	for k := range l.Raw {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, k := range keys {
		b = append(b, attr(uint16(k), l.Raw[uint16(k)])...)
	}
	return b
}

type countedStableDump struct {
	calls  int
	safety bool
	last   byte
}

func (d *countedStableDump) Request(_ context.Context, kind uint16, _ []byte, _ bool) ([][]byte, error) {
	d.calls++
	if kind != 18 {
		return nil, nil
	}
	d.last++
	l := LinkRecord{Index: 1, Name: "lo", Hardware: 772, MTU: 65536, Raw: map[uint16][]byte{3: []byte("lo\x00"), 4: u32(65536), 23: make([]byte, 200)}}
	l.Raw[23][0] = d.last
	if d.safety {
		l.Flags = uint32(d.calls)
	}
	return [][]byte{afLinkBodyTest(l)}, nil
}
func TestStableInventoryUsesPinnedComparisonReturnsOriginalSecondDump(t *testing.T) {
	d := &countedStableDump{}
	raw, e := ReadStableInventory(context.Background(), d, func() error { return nil })
	if e != nil || d.calls != 8 || len(raw.Links) != 1 || raw.Links[0].Raw[23][0] != 2 {
		t.Fatal("counter drift or masked return", e, d.calls)
	}
	d = &countedStableDump{safety: true}
	if _, e = ReadStableInventory(context.Background(), d, func() error { return nil }); e == nil || d.calls != 16 {
		t.Fatal("safety change accepted/retry", e, d.calls)
	}
}
func sourceAFWireTest(links []LinkRecord, mode string) NetlinkWire {
	return wireFunc(func(request []byte) ([]Datagram, error) {
		var all []byte
		for _, l := range links {
			body := afLinkBodyTest(l)
			b := make([]byte, 16+len(body))
			binary.NativeEndian.PutUint32(b, uint32(len(b)))
			binary.NativeEndian.PutUint16(b[4:], 16)
			binary.NativeEndian.PutUint16(b[6:], 2)
			copy(b[8:16], request[8:16])
			copy(b[16:], body)
			all = append(all, b...)
		}
		done := make([]byte, 20)
		binary.NativeEndian.PutUint32(done, 20)
		binary.NativeEndian.PutUint16(done[4:], 3)
		copy(done[8:16], request[8:16])
		all = append(all, done...)
		d := Datagram{Bytes: all}
		switch mode {
		case "sender":
			d.SenderPID = 9
		case "seq":
			d.Bytes[8]++
		case "port":
			d.Bytes[12]++
		case "truncated":
			d.Truncated = true
		case "done":
			d.Bytes = d.Bytes[:len(d.Bytes)-20]
		}
		return []Datagram{d}, nil
	})
}
func TestRouteAFProducerRecordedWireClaimsAndLifetime(t *testing.T) {
	raw, _ := recordedRaw(t)
	for _, mode := range []string{"ok", "sender", "seq", "port", "truncated", "done"} {
		owner := &afSourcePassOwner{controller: &operation1Controller{}, active: true, port: 77}
		pass, e := collectRouteAFPass(context.Background(), owner, sourceAFWireTest(raw.Links, mode))
		if mode != "ok" {
			if e == nil || owner.last != nil {
				t.Fatal("accepted", mode)
			}
			continue
		}
		if e != nil || pass.reader != nil || len(pass.headers) == 0 || len(pass.request) != 32 {
			t.Fatal("source provenance", e)
		}
		if _, e = pass.requestClaim(); e != nil {
			t.Fatal(e)
		}
		owner.active = false
		if _, e = pass.requestClaim(); e == nil {
			t.Fatal("expired source pass")
		}
	}
}
func TestPrecreationConversionOnlyAndOneUse(t *testing.T) {
	for _, mode := range []string{"ok", "phase", "attempt", "sequence", "generation", "later-session", "target", "pass-port", "policy"} {
		t.Run(mode, func(t *testing.T) {
			c, f := operation1AdapterRig(t)
			s := c.adapter.session
			s.canonical = nil
			s.plan = nil
			s.createAttempted = false
			s.forwardStep = 0
			c.tx.handle = nil
			c.tx.Ready = false
			c.tx.plan = f.p
			baseline := *s.baseline
			baseline.raw = cloneRawExact(s.baseline.raw)
			baseline.sys = map[string]string(cloneSysExact(SysctlSnapshot(s.baseline.sys)))
			for _, path := range effectiveV4Paths() {
				baseline.sys[path] = "0"
			}
			s.baseline = &baseline
			raw := cloneRawExact(baseline.raw)
			sys := cloneSysExact(SysctlSnapshot(baseline.sys))
			po := &afSourcePassOwner{controller: c, active: true, port: 77}
			pass, e := collectRouteAFPass(context.Background(), po, sourceAFWireTest(raw.Links, "ok"))
			if e != nil {
				t.Fatal(e)
			}
			pass.sys = cloneSysExact(sys)
			pass.effective, e = parseEffectiveV4(sys)
			if e != nil {
				t.Fatal(e)
			}
			c.precreationSequence = 1
			c.precreationGeneration = 1
			o := &precreationObservation{owner: c, baseline: &baseline, pass: pass, raw: raw, sys: sys, generation: 1, sequence: 1, phase: "precreation", identity: s.identity, binding: c.tx.plan.Binding}
			switch mode {
			case "phase":
				o.phase = "created"
			case "attempt":
				c.attempt = &originalAttempt{owner: c}
			case "sequence":
				o.sequence++
			case "generation":
				o.generation++
			case "later-session":
				h, _ := Attach(f.p, 7)
				s.canonical = &h
			case "target":
				o.raw.Links = append(o.raw.Links, LinkRecord{Name: f.p.Binding.Name})
			case "pass-port":
				po.port++
			case "policy":
				o.sys["/proc/sys/net/ipv4/ip_forward"] = "1"
			}
			inventory, e := consumePrecreation(c, o)
			if mode == "ok" {
				if e != nil || len(inventory.Links) != len(raw.Links) || c.tx.Ready {
					t.Fatal("precreation refused/Ready", e)
				}
			} else if e == nil {
				t.Fatal("unsupported promoted", mode)
			}
			if _, e = consumePrecreation(c, o); e == nil {
				t.Fatal("observation reused")
			}
		})
	}
}
