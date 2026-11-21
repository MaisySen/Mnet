//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"maps"
	"reflect"
	"slices"
)

// NO implementation outside _test.go. Structural source boundary, no live wire,
// syscall reader, FD or callbacks with real effects are added. This is not auth.
type inertAFSource interface {
	NetlinkWire
	readAFWitnesses(context.Context, []string) (SysctlSnapshot, error)
	inertAFSourceOnly()
}
type afReplyHeader struct {
	Length              uint32
	Type, Flags         uint16
	Sequence, Recipient uint32
	Sender              uint32
	Truncated           bool
}
type afReadPass struct {
	request   []byte
	replies   []Datagram
	headers   []afReplyHeader
	links     []LinkRecord
	sys       SysctlSnapshot
	sequence  uint32
	reader    *inertAFReader
	effective *effectiveV4Observation
}
type inertAFReader struct {
	source   inertAFSource
	localPID uint32
	sequence uint32
	last     *afReadPass
}
type afRecordingWire struct {
	source  inertAFSource
	request []byte
	replies []Datagram
}

func (w *afRecordingWire) Exchange(ctx context.Context, request []byte) ([]Datagram, error) {
	w.request = slices.Clone(request)
	ds, e := w.source.Exchange(ctx, request)
	for _, d := range ds {
		d.Bytes = slices.Clone(d.Bytes)
		w.replies = append(w.replies, d)
	}
	return ds, e
}
func (r *inertAFReader) read(ctx context.Context) (*afReadPass, error) {
	return r.readPass(ctx, false)
}

// Source-only extended collector. Legacy pass cannot masquerade as this profile.
func (r *inertAFReader) readEffective(ctx context.Context) (*afReadPass, error) {
	return r.readPass(ctx, true)
}
func (r *inertAFReader) readPass(ctx context.Context, effective bool) (*afReadPass, error) {
	if r == nil {
		return nil, afRequest
	}
	r.last = nil
	if r.source == nil || r.localPID == 0 || ctx == nil || ctx.Err() != nil || r.sequence == ^uint32(0) {
		return nil, afRequest
	}
	// Any new pass invalidates prior pass, even if the new collection fails.
	r.last = nil
	w := &afRecordingWire{source: r.source}
	c := NewInMemoryNetlinkClient(w, r.localPID)
	c.sequence = r.sequence
	bodies, e := c.requestFixture(ctx, 18, make([]byte, 16), true)
	r.sequence = c.sequence
	if e != nil {
		return nil, e
	}
	if len(bodies) == 0 || len(bodies) > 4096 {
		return nil, afShape
	}
	p := &afReadPass{request: slices.Clone(w.request), replies: w.replies, sequence: r.sequence, reader: r}
	paths := []string{"/proc/sys/net/ipv4/ip_forward"}
	for _, name := range []string{"all", "default"} {
		paths = append(paths, "/proc/sys/net/ipv4/conf/"+name+"/forwarding", ipv6DirPrefix+name)
		for _, leaf := range []string{"forwarding", "force_forwarding", "disable_ipv6", "addr_gen_mode"} {
			paths = append(paths, "/proc/sys/net/ipv6/conf/"+name+"/"+leaf)
		}
	}
	for _, body := range bodies {
		l, e := DecodeLink(body)
		if e != nil || !safeInterfaceName(l.Name) {
			return nil, afShape
		}
		p.links = append(p.links, l)
		paths = append(paths, "/proc/sys/net/ipv4/conf/"+l.Name+"/forwarding")
		applicable, e := LinkIPv6Applicable(l)
		if e != nil {
			return nil, e
		}
		paths = append(paths, ipv6DirPrefix+l.Name)
		if applicable {
			for _, leaf := range []string{"forwarding", "force_forwarding", "disable_ipv6", "addr_gen_mode"} {
				paths = append(paths, "/proc/sys/net/ipv6/conf/"+l.Name+"/"+leaf)
			}
		}
	}
	if effective {
		paths = append(paths, effectiveV4Paths()...)
	}
	// Same reader call/pass, after the received dump. Not atomic/stable kernel
	// proof. No caller can substitute a sys map after the pass is built.
	sys, e := r.source.readAFWitnesses(ctx, slices.Clone(paths))
	if e != nil || ctx.Err() != nil {
		return nil, afPolicy
	}
	for _, path := range paths {
		if _, ok := sys[path]; !ok {
			return nil, afPolicy
		}
	}
	p.sys = maps.Clone(sys)
	if effective {
		p.effective, e = parseEffectiveV4(p.sys)
		if e != nil {
			return nil, e
		}
	}
	for _, d := range p.replies {
		b := d.Bytes
		for len(b) > 0 {
			n := int(binary.NativeEndian.Uint32(b))
			p.headers = append(p.headers, afReplyHeader{Length: uint32(n), Type: binary.NativeEndian.Uint16(b[4:]), Flags: binary.NativeEndian.Uint16(b[6:]), Sequence: binary.NativeEndian.Uint32(b[8:]), Recipient: binary.NativeEndian.Uint32(b[12:]), Sender: d.SenderPID, Truncated: d.Truncated})
			b = b[(n+3)&^3:]
		}
	}
	r.last = p
	return p, nil
}
func (p *afReadPass) requestClaim() (ownedAFRequestClaim, error) {
	if p == nil || p.reader == nil || p.reader.last != p || p.sequence != p.reader.sequence || len(p.request) != 32 {
		return ownedAFRequestClaim{}, afRequest
	}
	b := p.request
	if binary.NativeEndian.Uint32(b) != 32 || binary.NativeEndian.Uint16(b[4:]) != 18 || binary.NativeEndian.Uint16(b[6:]) != 0x301 || binary.NativeEndian.Uint32(b[8:]) != p.sequence || binary.NativeEndian.Uint32(b[12:]) != p.reader.localPID || !reflect.DeepEqual(b[16:], make([]byte, 16)) {
		return ownedAFRequestClaim{}, afRequest
	}
	return ownedAFRequestClaim{Flags: 0x301, Shape: "full-v4c33-v6c60-ip38-icmp7"}, nil
}
func (p *afReadPass) link(index int) (LinkRecord, error) {
	if _, e := p.requestClaim(); e != nil {
		return LinkRecord{}, e
	}
	var found *LinkRecord
	for _, l := range p.links {
		if l.Index == index {
			if found != nil {
				return LinkRecord{}, afShape
			}
			copy := l
			found = &copy
		}
	}
	if found == nil {
		return LinkRecord{}, afShape
	}
	out := *found
	out.Raw = cloneRawAttrs(out.Raw)
	return out, nil
}

// Explicitly diagnostic: not a policy-safe Inventory or readiness credential.
type ownedAFDiagnostic struct {
	parsed  ownedAFParsedProposal
	phase   ownedAFPhase
	request []byte
	headers []afReplyHeader
}
type ownedAFDisabledCandidate struct {
	pass             *afReadPass
	session          *adapterSession
	diagnostic       ownedAFDiagnostic
	uncheckedDevconf bool
}

func (s *adapterSession) bindCreationApplicability(p *afReadPass) error {
	if s == nil || s.canonical == nil || s.forwardStep != 1 || s.canonical.TagConfirmed || s.pending != nil || s.cleanupMode || s.poisoned || s.afCreationBound {
		return afPhase
	}
	l, e := p.link(s.canonical.Link.Index)
	if e != nil {
		return e
	}
	if !handleLinkMatches(*s.canonical, Link{Index: l.Index, Name: l.Name, Alias: l.Alias, Kind: "tun", MTU: l.MTU}) {
		return afPhase
	}
	kind, e := linkKind(l)
	if e != nil || kind != "tun" {
		return afPhase
	}
	applicable, e := LinkIPv6Applicable(l)
	if e != nil {
		return e
	}
	if e = validateIPv6Directory(l.Name, applicable, p.sys); e != nil {
		return afDirectory
	}
	request, e := p.requestClaim()
	if e != nil {
		return e
	}
	parsed, e := parseOwnedAFFullProposal(l, p.sys, request, ownedAFPhaseClaim{Phase: afCreated, IPv6ApplicableAtCreation: applicable})
	if e != nil {
		return e
	}
	s.afCreationBound = true
	s.afCreationApplicable = applicable
	s.afCreationAttrs = cloneRawAttrs(parsed.raw)
	return nil
}
func (s *adapterSession) diagnoseOwnedAF(p *afReadPass) (ownedAFDiagnostic, error) {
	var zero ownedAFDiagnostic
	if s == nil || !s.afCreationBound || s.canonical == nil || s.closeAttempted {
		return zero, afPhase
	}
	l, e := p.link(s.canonical.Link.Index)
	if e != nil {
		return zero, e
	}
	if !handleLinkMatches(*s.canonical, Link{Index: l.Index, Name: l.Name, Alias: l.Alias, Kind: "tun", MTU: l.MTU}) {
		return zero, afPhase
	}
	kind, e := linkKind(l)
	if e != nil || kind != "tun" {
		return zero, afPhase
	}
	phase := afCreated
	switch {
	case s.cleanupMode || s.poisoned:
		phase = afCleanup
	case s.pending != nil && s.pending.Kind == "disable-owned-link-ipv6":
		phase = afDisablePending
	case s.forwardStep >= 3:
		phase = afDisabled
	case s.canonical.TagConfirmed:
		phase = afTagged
	}
	request, e := p.requestClaim()
	if e != nil {
		return zero, e
	}
	parsed, e := parseOwnedAFFullProposal(l, p.sys, request, ownedAFPhaseClaim{Phase: phase, IPv6ApplicableAtCreation: s.afCreationApplicable})
	if e != nil {
		return zero, e
	}
	return ownedAFDiagnostic{parsed: parsed, phase: phase, request: slices.Clone(p.request), headers: slices.Clone(p.headers)}, nil
}
func (s *adapterSession) disabledAFCandidate(p *afReadPass) (ownedAFDisabledCandidate, error) {
	d, e := s.diagnoseOwnedAF(p)
	if e != nil {
		return ownedAFDisabledCandidate{}, e
	}
	if d.phase != afDisabled || !d.parsed.applicable || !d.parsed.disabled {
		return ownedAFDisabledCandidate{}, afPhase
	}
	// Unchecked IPv4 AND IPv6 devconf words are retained, not deemed safe. This
	// candidate cannot become Inventory/active observation until policy review.
	return ownedAFDisabledCandidate{diagnostic: d, uncheckedDevconf: true, pass: p, session: s}, nil
}

// Snapshot boundary: never hand callers the pass's link slice/maps/bytes.
func (p *afReadPass) linkSnapshot() ([]LinkRecord, error) {
	if _, e := p.requestClaim(); e != nil {
		return nil, e
	}
	return cloneRawExact(RawInventory{Links: p.links}).Links, nil
}
