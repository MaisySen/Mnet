//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"reflect"
	"testing"
)

func featureBitmapTest(v uint64, masked bool) []byte {
	b := make([]byte, 8)
	binary.NativeEndian.PutUint32(b, uint32(v))
	binary.NativeEndian.PutUint32(b[4:], uint32(v>>32))
	raw := append(attr(2, u32(64)), attr(4, b)...)
	if masked {
		raw = append(raw, attr(5, []byte{255, 255, 255, 255, 255, 255, 255, 255})...)
	} else {
		raw = append(raw, attr(1, nil)...)
	}
	return raw
}
func featureReplyTest(req ethtoolFeatureRequest, active uint64) Datagram {
	header := append(attr(1, u32(req.ifindex)), attr(2, append([]byte(req.name), 0))...)
	attrs := attr(0x8001, header)
	for n, v := range []uint64{1<<0 | 1<<3 | 1<<16, 1 << 3, active, 0} {
		attrs = append(attrs, attr(uint16(n+2)|0x8000, featureBitmapTest(v, n == 0))...)
	}
	b := make([]byte, 20+len(attrs))
	binary.NativeEndian.PutUint32(b, uint32(len(b)))
	binary.NativeEndian.PutUint16(b[4:], req.family)
	binary.NativeEndian.PutUint32(b[8:], req.sequence)
	binary.NativeEndian.PutUint32(b[12:], req.recipient)
	b[16] = 11
	b[17] = 1
	copy(b[20:], attrs)
	return Datagram{Bytes: b}
}
func testFeatureReq() ethtoolFeatureRequest {
	return ethtoolFeatureRequest{family: 37, sequence: 9, recipient: 77, ifindex: 11, name: "mno-fixture", profile: pinnedFeaturesV617}
}
func nestedTunTest() []byte {
	var data []byte
	for _, n := range []uint16{3, 4, 5, 6, 7} {
		v := byte(0)
		if n == 3 {
			v = 1
		}
		data = append(data, attr(n, []byte{v})...)
	}
	return append(attr(1, []byte{'t', 'u', 'n', 0}), attr(0x8002, data)...)
}
func TestTUNNestedFramingExactSourceSchema(t *testing.T) {
	valid := nestedTunTest()
	if _, e := parseTUNNested(LinkRecord{Raw: map[uint16][]byte{18: valid}}); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"tap", "pi", "vnet", "persist", "multi", "missing", "duplicate", "unknown", "unflagged", "netorder", "short", "padding"} {
		t.Run(mode, func(t *testing.T) {
			info, _ := schemaAttrs(valid, map[uint16]bool{1: false, 2: true})
			data := append([]byte(nil), info[2]...)
			switch mode {
			case "tap":
				data[4] = 2
			case "pi":
				data[12] = 1
			case "vnet":
				data[20] = 1
			case "persist":
				data[28] = 1
			case "multi":
				data[36] = 1
			case "missing":
				data = data[:len(data)-8]
			case "duplicate":
				data = append(data, attr(4, []byte{0})...)
			case "unknown":
				data = append(data, attr(8, u32(1))...)
			case "short":
				data[0] = 3
			case "padding":
				data[5] = 1
			}
			raw := append(attr(1, info[1]), attr(0x8002, data)...)
			if mode == "unflagged" {
				binary.NativeEndian.PutUint16(raw[10:], 2)
			}
			if mode == "netorder" {
				binary.NativeEndian.PutUint16(raw[10:], 0xc002)
			}
			if _, e := parseTUNNested(LinkRecord{Raw: map[uint16][]byte{18: raw}}); e == nil {
				t.Fatal(mode)
			}
		})
	}
}
func TestEthtoolCompactActiveNotHardwareOrWanted(t *testing.T) {
	r := testFeatureReq()
	d := featureReplyTest(r, 1<<0|1<<11|1<<14)
	f, e := parseEthtoolActive(d, r)
	if e != nil || f.hardware&(1<<3) == 0 || f.wanted&(1<<3) == 0 || !f.rawOutputCompatible() {
		t.Fatal(f, e)
	}
	for _, n := range []uint{1, 3, 4, 16, 18, 20, 26, 27, 32, 33, 34, 35} {
		f, e = parseEthtoolActive(featureReplyTest(r, 1<<n), r)
		if e != nil || f.rawOutputCompatible() {
			t.Fatal("offload", n, e)
		}
	}
}
func TestEthtoolBoundedReplyIdentityRefusals(t *testing.T) {
	for _, mode := range []string{"sender", "truncated", "family", "sequence", "recipient", "multipart", "command", "version", "reserved", "length", "huge", "ifindex", "name", "profile", "nested-flags", "duplicate", "unknown", "reserved-feature"} {
		t.Run(mode, func(t *testing.T) {
			r := testFeatureReq()
			d := featureReplyTest(r, 0)
			switch mode {
			case "sender":
				d.SenderPID = 9
			case "truncated":
				d.Truncated = true
			case "family":
				r.family++
			case "sequence":
				r.sequence++
			case "recipient":
				r.recipient++
			case "multipart":
				binary.NativeEndian.PutUint16(d.Bytes[6:], 2)
			case "command":
				d.Bytes[16]++
			case "version":
				d.Bytes[17]++
			case "reserved":
				d.Bytes[18] = 1
			case "length":
				d.Bytes = d.Bytes[:len(d.Bytes)-1]
			case "huge":
				d.Bytes = make([]byte, 4097)
			case "ifindex":
				r.ifindex++
			case "name":
				r.name += "x"
			case "profile":
				r.profile = "unknown-kernel"
			case "nested-flags":
				binary.NativeEndian.PutUint16(d.Bytes[22:], 1)
			case "duplicate":
				d.Bytes = append(d.Bytes, attr(0x8001, nil)...)
				binary.NativeEndian.PutUint32(d.Bytes, uint32(len(d.Bytes)))
			case "unknown":
				d.Bytes = append(d.Bytes, attr(0x8006, nil)...)
				binary.NativeEndian.PutUint32(d.Bytes, uint32(len(d.Bytes)))
			case "reserved-feature":
				d = featureReplyTest(r, 1<<2)
			}
			if _, e := parseEthtoolActive(d, r); e == nil {
				t.Fatal(mode)
			}
		})
	}
	for _, raw := range [][]byte{featureBitmapTest(0, false)[:8], append(featureBitmapTest(0, false), attr(5, make([]byte, 8))...)} {
		if _, e := compactFeatureBitmap(raw, false); e == nil {
			t.Fatal("bad bitmap")
		}
	}
	raw := featureBitmapTest(0, false)
	binary.NativeEndian.PutUint32(raw[4:], 65)
	if _, e := compactFeatureBitmap(raw, false); e == nil {
		t.Fatal("wrong profile size")
	}
}

type heldMemoryWire struct {
	id           heldPacketIdentity
	link         LinkRecord
	calls        int
	replaceAfter bool
	request      []byte
}

func (*heldMemoryWire) inertHeldPacketWireOnly() {}
func (w *heldMemoryWire) heldIdentity(context.Context) (heldPacketIdentity, error) {
	w.calls++
	id := w.id
	if w.replaceAfter && w.calls == 2 {
		id.fdObject = &packetOriginalObject{serial: id.fdObject.serial}
	}
	return id, nil
}
func (w *heldMemoryWire) heldLink(context.Context) (LinkRecord, error) { return w.link, nil }
func (w *heldMemoryWire) heldFeatures(_ context.Context, request []byte) (Datagram, error) {
	w.request = append([]byte(nil), request...)
	r := ethtoolFeatureRequest{family: binary.NativeEndian.Uint16(request[4:]), sequence: binary.NativeEndian.Uint32(request[8:]), recipient: binary.NativeEndian.Uint32(request[12:]), ifindex: uint32(w.link.Index), name: w.link.Name, profile: pinnedFeaturesV617}
	return featureReplyTest(r, 1<<0|1<<11|1<<14), nil
}
func heldSupplierRig(t *testing.T) (*heldPacketReader, *inertAFReader, *fullAFMemorySource, *heldMemoryWire) {
	t.Helper()
	s, ar, source := effectiveRig(t)
	source.link.Hardware = 65534
	source.link.Flags = 0x1091
	source.link.Raw[18] = nestedTunTest()
	id := heldPacketIdentity{fdObject: &packetOriginalObject{serial: 9}, generation: 1, target: s.identity.Namespace, device: s.identity.Namespace, identity: s.identity, handle: *s.canonical}
	ledger, e := newHeldPacketLedger(id)
	if e != nil {
		t.Fatal(e)
	}
	w := &heldMemoryWire{id: id, link: source.link}
	r := &heldPacketReader{wire: w, ledger: ledger, family: 37, recipient: 77, profile: pinnedFeaturesV617}
	return r, ar, source, w
}
func TestHeldSupplierBracketsOriginalIdentityAndALLPass(t *testing.T) {
	r, ar, _, w := heldSupplierRig(t)
	p, e := ar.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	o, e := r.read(context.Background(), p)
	if e != nil || !o.current() || len(o.effective) != 33 || !o.features.rawOutputCompatible() || w.calls != 2 {
		t.Fatal(o, e)
	}
	if w.request[16] != 11 || w.request[17] != 1 || binary.NativeEndian.Uint16(w.request[6:]) != 1 {
		t.Fatal("not read-only GET")
	}
	// Source mutation doesn't rewrite frozen observation, new pass invalidates it.
	before := cloneRawAttrs(o.link.Raw)
	w.link.Raw = cloneRawAttrs(w.link.Raw)
	w.link.Raw[18][4] ^= 1
	if !reflect.DeepEqual(before, o.link.Raw) {
		t.Fatal("frozen record alias")
	}
	if _, e = r.read(context.Background(), p); e == nil || o.current() {
		t.Fatal("failed new pass kept old")
	}
}
func TestHeldSupplierGenerationAndNamespaceRefusals(t *testing.T) {
	for _, kind := range []string{"ioctl-intent", "duplicate-intent", "export-intent", "close-intent", "namespace-change", "custody-loss"} {
		r, ar, _, _ := heldSupplierRig(t)
		p, e := ar.readEffective(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		o, e := r.read(context.Background(), p)
		if e != nil {
			t.Fatal(e)
		}
		if e = r.ledger.invalidate(kind); e != nil || o.current() {
			t.Fatal(kind, e)
		}
		if _, e = r.read(context.Background(), p); e == nil {
			t.Fatal("lineage regained", kind)
		}
	}
	r, ar, _, w := heldSupplierRig(t)
	w.replaceAfter = true
	p, e := ar.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.read(context.Background(), p); e == nil {
		t.Fatal("same serial replacement")
	}
	r, ar, _, w = heldSupplierRig(t)
	w.id.device.Inode++
	p, e = ar.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.read(context.Background(), p); e == nil {
		t.Fatal("device netns differs")
	}
}

func TestCorrectedBOOTPAndDirectORConsumerOperands(t *testing.T) {
	s, r, source := effectiveRig(t)
	source.sys["/proc/sys/net/ipv4/conf/all/bootp_relay"] = "1"
	setEffectiveLocal(t, source, 9, 1)
	for _, n := range []int{14, 15, 29, 30} {
		source.sys["/proc/sys/net/ipv4/conf/all/"+effectiveV4Leaves[n]] = "1"
	}
	p, e := r.readEffective(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	words, e := p.effectiveV4(s.canonical.Link.Index)
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range []int{9, 14, 15, 29, 30} {
		if !words[n].Computed || !words[n].AllObserved || words[n].Value != 1 {
			t.Fatal(n, words[n])
		}
	}
	delete(source.sys, "/proc/sys/net/ipv4/conf/all/bootp_relay")
	if _, e = r.readEffective(context.Background()); e == nil {
		t.Fatal("missing BOOTP operand accepted")
	}
}
