//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sort"
	"testing"
)

func fullAFSynthetic() (LinkRecord, SysctlSnapshot) {
	l := LinkRecord{Index: 11, Name: "mno-fixture", MTU: 1500, Raw: map[uint16][]byte{}}
	c4 := make([]byte, 132)
	c6 := make([]byte, 240)
	binary.NativeEndian.PutUint32(c6[26*4:], 1)
	binary.NativeEndian.PutUint32(c6[47*4:], 1)
	stats := make([]byte, 304)
	binary.NativeEndian.PutUint64(stats, 38)
	icmp := make([]byte, 56)
	binary.NativeEndian.PutUint64(icmp, 7)
	var v6 []byte
	for _, a := range []struct {
		k uint16
		b []byte
	}{{1, make([]byte, 4)}, {2, c6}, {3, stats}, {5, make([]byte, 16)}, {6, icmp}, {7, make([]byte, 16)}, {8, []byte{1}}} {
		v6 = append(v6, attr(a.k, a.b)...)
	}
	l.Raw[26] = append(attr(2, attr(1, c4)), attr(10, v6)...)
	l.Raw[3] = append([]byte(l.Name), 0)
	l.Raw[4] = u32(l.MTU)
	l.Raw[18] = attr(1, []byte("tun\x00"))
	p := "/proc/sys/net/ipv6/conf/" + l.Name + "/"
	sys := SysctlSnapshot{ipv6DirPrefix + l.Name: "present", "/proc/sys/net/ipv4/conf/" + l.Name + "/forwarding": "0", p + "forwarding": "0", p + "force_forwarding": "0", p + "disable_ipv6": "1", p + "addr_gen_mode": "1"}
	return l, sys
}
func fullRequestClaim() ownedAFRequestClaim {
	return ownedAFRequestClaim{Flags: 0x301, Shape: "full-v4c33-v6c60-ip38-icmp7"}
}
func mutateFull(l *LinkRecord, change func(map[uint16][]byte, map[uint16][]byte)) {
	families, _ := decodeAttributes(l.Raw[26])
	a4, _ := decodeAttributes(families[2])
	a6, _ := decodeAttributes(families[10])
	change(a4, a6)
	encode := func(attrs map[uint16][]byte) []byte {
		var keys []int
		for k := range attrs {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		var out []byte
		for _, k := range keys {
			out = append(out, attr(uint16(k), attrs[uint16(k)])...)
		}
		return out
	}
	l.Raw[26] = append(attr(2, encode(a4)), attr(10, encode(a6))...)
}
func TestFullAFProposalDecisiveRefusals(t *testing.T) {
	for _, mode := range []string{"count59", "count61", "force-word", "stats-count", "icmp-count", "unknown-attr", "unknown-family", "unknown-flag", "padding", "ra-mtu-zero", "missing-witness", "disabled-flags", "disable-disagree", "directory", "v4-count", "v4-forward", "unknown-request", "applicability"} {
		t.Run(mode, func(t *testing.T) {
			l, s := fullAFSynthetic()
			request := fullRequestClaim()
			phase := ownedAFPhaseClaim{Phase: afDisabled, IPv6ApplicableAtCreation: true}
			want := afShape
			mutateFull(&l, func(v4, v6 map[uint16][]byte) {
				switch mode {
				case "count59":
					v6[2] = v6[2][:236]
				case "count61":
					v6[2] = append(v6[2], 0, 0, 0, 0)
				case "force-word":
					binary.NativeEndian.PutUint32(v6[2][59*4:], 1)
					want = afPolicy
				case "stats-count":
					binary.NativeEndian.PutUint64(v6[3], 37)
				case "icmp-count":
					binary.NativeEndian.PutUint64(v6[6], 6)
				case "unknown-attr":
					v6[77] = nil
				case "unknown-flag":
					binary.NativeEndian.PutUint32(v6[1], 1)
				case "ra-mtu-zero":
					v6[9] = make([]byte, 4)
				case "disabled-flags":
					binary.NativeEndian.PutUint32(v6[1], 0x80000000)
					want = afPhase
				case "disable-disagree":
					s["/proc/sys/net/ipv6/conf/"+l.Name+"/disable_ipv6"] = "0"
					want = afPolicy
				case "v4-count":
					v4[1] = append(v4[1], 0, 0, 0, 0)
				case "v4-forward":
					binary.NativeEndian.PutUint32(v4[1], 1)
					want = afPolicy
				}
			})
			switch mode {
			case "unknown-family":
				l.Raw[26] = append(l.Raw[26], attr(77, nil)...)
			case "padding":
				l.Raw[26] = append(l.Raw[26], []byte{5, 0, 77, 0, 1, 1, 0, 0}...)
			case "missing-witness":
				delete(s, "/proc/sys/net/ipv6/conf/"+l.Name+"/force_forwarding")
				want = afPolicy
			case "directory":
				s[ipv6DirPrefix+l.Name] = "ENOENT"
				want = afDirectory
			case "unknown-request":
				request.Flags = 0x305
				want = afRequest
			case "applicability":
				phase.IPv6ApplicableAtCreation = false
				want = afPhase
			}
			if _, e := parseOwnedAFFullProposal(l, s, request, phase); !errors.Is(e, want) {
				t.Fatal(mode, e, want)
			}
		})
	}
}
func TestFullAFRecordedShapeDiagnosticOnly(t *testing.T) {
	raw, s := recordedRaw(t)
	var l LinkRecord
	for _, candidate := range raw.Links {
		if candidate.Name == "gretap0" {
			l = candidate
		}
	}
	if l.Index == 0 {
		t.Fatal("recorded source anchor disappeared")
	}
	families, _ := decodeAttributes(l.Raw[26])
	v6, _ := decodeAttributes(families[10])
	if len(v6[2]) != 240 {
		t.Fatal("recorded conf shape changed")
	}
	// This added value is synthetic fixture evidence, not a historical sysctl read.
	s = maps.Clone(s)
	s["/proc/sys/net/ipv6/conf/"+l.Name+"/force_forwarding"] = "0"
	before := cloneRawAttrs(l.Raw)
	d, e := parseOwnedAFFullProposal(l, s, fullRequestClaim(), ownedAFPhaseClaim{Phase: afCreated, IPv6ApplicableAtCreation: true})
	if e != nil {
		t.Fatal("recorded shape diagnostic", e)
	}
	if !reflect.DeepEqual(before, l.Raw) || !reflect.DeepEqual(before, d.raw) {
		t.Fatal("raw normalized or discarded")
	}
}

type fullAFMemorySource struct {
	link          LinkRecord
	sys           SysctlSnapshot
	request       []byte
	paths         []string
	corruptHeader bool
	omitForce     bool
}

func (*fullAFMemorySource) inertAFSourceOnly() {}
func (f *fullAFMemorySource) Exchange(_ context.Context, r []byte) ([]Datagram, error) {
	f.request = slices.Clone(r)
	body := make([]byte, 16)
	binary.NativeEndian.PutUint16(body[2:], f.link.Hardware)
	binary.NativeEndian.PutUint32(body[4:], uint32(f.link.Index))
	binary.NativeEndian.PutUint32(body[8:], f.link.Flags)
	var keys []int
	for k := range f.link.Raw {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, k := range keys {
		body = append(body, attr(uint16(k), f.link.Raw[uint16(k)])...)
	}
	frame := make([]byte, 16+len(body))
	binary.NativeEndian.PutUint32(frame, uint32(len(frame)))
	binary.NativeEndian.PutUint16(frame[4:], 16)
	binary.NativeEndian.PutUint16(frame[6:], 2)
	copy(frame[8:16], r[8:16])
	copy(frame[16:], body)
	done := make([]byte, 20)
	binary.NativeEndian.PutUint32(done, 20)
	binary.NativeEndian.PutUint16(done[4:], 3)
	copy(done[8:16], r[8:16])
	if f.corruptHeader {
		frame[8]++
	}
	return []Datagram{{Bytes: frame}, {Bytes: done}}, nil
}
func (f *fullAFMemorySource) readAFWitnesses(_ context.Context, paths []string) (SysctlSnapshot, error) {
	f.paths = slices.Clone(paths)
	s := maps.Clone(f.sys)
	if f.omitForce {
		delete(s, "/proc/sys/net/ipv6/conf/"+f.link.Name+"/force_forwarding")
	}
	return s, nil
}
func TestFullAFReaderOwnsExactPassProvenance(t *testing.T) {
	l, sys := fullAFSynthetic()
	source := &fullAFMemorySource{link: l, sys: sys}
	r := &inertAFReader{source: source, localPID: 77}
	p, e := r.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(p.request) != 32 || binary.NativeEndian.Uint16(p.request[6:]) != 0x301 || !reflect.DeepEqual(p.request[16:], make([]byte, 16)) || len(p.headers) != 2 {
		t.Fatal("provenance incomplete")
	}
	force := "/proc/sys/net/ipv6/conf/" + l.Name + "/force_forwarding"
	if !slices.Contains(source.paths, force) || p.sys[force] != "0" {
		t.Fatal("same-pass force witness not requested")
	}
	if p.headers[0].Type != 16 || p.headers[1].Type != 3 || p.headers[0].Sequence != p.sequence || p.headers[0].Recipient != 77 || p.headers[0].Sender != 0 {
		t.Fatal("reply header not retained")
	}
	source.request[0] = 0
	source.sys[force] = "1"
	if p.request[0] != 32 || p.sys[force] != "0" {
		t.Fatal("pass aliased source")
	}
	source.sys[force] = "0"
	if _, e = r.read(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = p.requestClaim(); !errors.Is(e, afRequest) {
		t.Fatal("old pass reused", e)
	}
	source.corruptHeader = true
	if _, e = r.read(context.Background()); e == nil {
		t.Fatal("reply sequence mismatch accepted")
	}
	source.corruptHeader = false
	source.omitForce = true
	if _, e = r.read(context.Background()); !errors.Is(e, afPolicy) {
		t.Fatal("missing same-pass witness accepted", e)
	}
}
func TestLedgerCreationApplicabilityCannotBeCallerPromoted(t *testing.T) {
	_, a, _ := protocolRig(t)
	base := a.session.baseline
	i, e := base.Compose(base.raw, SysctlSnapshot(base.sys), nil, false)
	if e != nil {
		t.Fatal(e)
	}
	binding, _, now := rig()
	binding.Namespace = base.ns
	p, e := Prepare(binding, i, now)
	if e != nil {
		t.Fatal(e)
	}
	h, e := AttachCreated(p, 11, 1500)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.session.bindCreated(p, h); e != nil {
		t.Fatal(e)
	}
	l, sys := fullAFSynthetic()
	l.Name = h.CreationLink.Name
	l.Alias = ""
	l.MTU = h.CreationLink.MTU
	l.Raw[3] = append([]byte(l.Name), 0)
	l.Raw[4] = u32(l.MTU)
	// Rename synthetic witness keys explicitly to the bound creation name.
	renamed := SysctlSnapshot{}
	for path, v := range sys {
		for n := 0; n+len("mno-fixture") <= len(path); n++ {
			if path[n:n+len("mno-fixture")] == "mno-fixture" {
				path = path[:n] + l.Name + path[n+len("mno-fixture"):]
				break
			}
		}
		renamed[path] = v
	}
	source := &fullAFMemorySource{link: l, sys: renamed}
	reader := &inertAFReader{source: source, localPID: 77}
	pass, e := reader.read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e = a.session.bindCreationApplicability(pass); e != nil {
		t.Fatal(e)
	}
	if !a.session.afCreationBound || !a.session.afCreationApplicable {
		t.Fatal("creation witness lost")
	}
	if e = a.session.bindCreationApplicability(pass); !errors.Is(e, afPhase) {
		t.Fatal("creation rebind allowed", e)
	}
	if _, e = a.session.disabledAFCandidate(pass); !errors.Is(e, afPhase) {
		t.Fatal("created diagnostic promoted to disabled", e)
	}
}

func TestFullAFPhaseDirectoryTable(t *testing.T) {
	for _, mode := range []string{"pre-disable-zero", "post-disable-zero", "absent-pre", "absent-post", "present-dir-absent", "absent-dir-present", "disabled-nonzero-flags"} {
		t.Run(mode, func(t *testing.T) {
			l, s := fullAFSynthetic()
			phase := ownedAFPhaseClaim{Phase: afCreated, IPv6ApplicableAtCreation: true}
			var want error
			switch mode {
			case "pre-disable-zero", "post-disable-zero":
				mutateFull(&l, func(_ map[uint16][]byte, v6 map[uint16][]byte) { binary.NativeEndian.PutUint32(v6[2][26*4:], 0) })
				s["/proc/sys/net/ipv6/conf/"+l.Name+"/disable_ipv6"] = "0"
				if mode == "post-disable-zero" {
					phase.Phase = afDisabled
					want = afPhase
				}
			case "absent-pre", "absent-post", "absent-dir-present":
				families, _ := decodeAttributes(l.Raw[26])
				l.Raw[26] = attr(2, families[2])
				phase.IPv6ApplicableAtCreation = false
				for _, leaf := range []string{"forwarding", "force_forwarding", "disable_ipv6", "addr_gen_mode"} {
					delete(s, "/proc/sys/net/ipv6/conf/"+l.Name+"/"+leaf)
				}
				s[ipv6DirPrefix+l.Name] = "ENOENT"
				if mode == "absent-post" {
					phase.Phase = afDisabled
					want = afPhase
				}
				if mode == "absent-dir-present" {
					s[ipv6DirPrefix+l.Name] = "present"
					want = afDirectory
				}
			case "present-dir-absent":
				s[ipv6DirPrefix+l.Name] = "ENOENT"
				want = afDirectory
			case "disabled-nonzero-flags":
				phase.Phase = afDisabled
				mutateFull(&l, func(_ map[uint16][]byte, v6 map[uint16][]byte) { binary.NativeEndian.PutUint32(v6[1], 0x80000000) })
				want = afPhase
			}
			d, e := parseOwnedAFFullProposal(l, s, fullRequestClaim(), phase)
			if want == nil {
				if e != nil {
					t.Fatal(mode, e)
				}
				if mode == "pre-disable-zero" && d.disabled {
					t.Fatal("pre-disable zero promoted")
				}
			} else if !errors.Is(e, want) {
				t.Fatal(mode, e, want)
			}
		})
	}
}
