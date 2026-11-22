//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"slices"
)

// Supported-encoding POLICY, not proof of architecture/config-specific emitter
// padding or nla_put_uint behavior. Unknown forms remain unsupported. Complete
// original bytes/order/type flags survive separately from semantic comparison.
type setupOuterRecord struct{ body []byte }

const setupOuterProfile = "fresh-down-tun-outer-bounded-v1"

// Only the known owned identity gets this decoder. All baseline/foreign links
// still use strict DecodeLink; no generic repeated-singleton exception.
func decodeSetupLink(body []byte, h Handle) (LinkRecord, error) {
	if len(body) < 16 {
		return LinkRecord{}, afShape
	}
	if int(int32(binary.NativeEndian.Uint32(body[4:]))) != h.Link.Index {
		return DecodeLink(body)
	}
	r, e := parseSetupOuterPhase(body, binary.NativeEndian.Uint32(body[8:])&1 != 0)
	if e != nil {
		return LinkRecord{}, e
	}
	if r.Name != h.Link.Name {
		return LinkRecord{}, afPhase
	}
	r.setupOuter = &setupOuterRecord{body: slices.Clone(body)}
	return r, nil
}
func parseSetupOuter(body []byte) (LinkRecord, error) { return parseSetupOuterPhase(body, false) }
func parseSetupOuterPhase(body []byte, up bool) (LinkRecord, error) {
	var r LinkRecord
	if len(body) < 16 || len(body) > 16384 || body[0] != 0 || body[1] != 0 || binary.NativeEndian.Uint32(body[12:]) != 0 {
		return r, afShape
	}
	r.Index = int(int32(binary.NativeEndian.Uint32(body[4:])))
	r.Hardware = binary.NativeEndian.Uint16(body[2:])
	r.Flags = binary.NativeEndian.Uint32(body[8:])
	if r.Index <= 0 || r.Hardware != 65534 || (!up && r.Flags != 0x1090) || (up && r.Flags != 0x110d1) {
		return r, afPhase
	}
	attrs := map[uint16][]byte{}
	raw := body[16:]
	pendingPad := false
	for len(raw) > 0 {
		if len(raw) < 4 {
			return r, afShape
		}
		n := int(binary.NativeEndian.Uint16(raw))
		typ := binary.NativeEndian.Uint16(raw[2:])
		aligned := (n + 3) &^ 3
		if n < 4 || n > len(raw) || aligned > len(raw) {
			return r, afShape
		}
		for _, v := range raw[n:aligned] {
			if v != 0 {
				return r, afShape
			}
		}
		if typ == 42 {
			// Bounded encoding policy: no payload/run, only before one of the two
			// held emitter's named 64-bit alignment sites. No assertion a PAD is
			// necessary/sufficient on a real kernel; missing helper proof stays open.
			if n != 4 || pendingPad {
				return r, afShape
			}
			pendingPad = true
			raw = raw[aligned:]
			continue
		}
		if pendingPad && typ != 23 && typ != 14 {
			return r, afShape
		}
		pendingPad = false
		key := typ
		if typ == 62|0x8000 || typ == 65|0x8000 {
			if n != 4 {
				return r, afShape
			}
			key = typ & 0x3fff
		} else if typ == 62 || typ == 65 || typ&0xc000 != 0 {
			return r, afShape
		}
		if _, yes := attrs[key]; yes {
			return r, afShape
		}
		attrs[key] = slices.Clone(raw[4:n])
		raw = raw[aligned:]
	}
	if pendingPad {
		return r, afShape
	}
	r.Raw = attrs
	var e error
	if r.Name, e = attrText(attrs, 3, true); e != nil || !safeInterfaceName(r.Name) {
		return r, afShape
	}
	if r.Alias, e = attrText(attrs, 20, false); e != nil {
		return r, afShape
	}
	if r.MTU, e = attrUint(attrs, 4, true); e != nil || r.MTU < 68 || r.MTU > 65535 {
		return r, afShape
	}
	// Required unconditional full-emitter operands. Presence of CONFIG_RPS32
	// is explicit and later retained, never interpreted as no RX capability.
	for _, k := range []uint16{3, 4, 7, 13, 14, 16, 17, 18, 23, 26, 27, 30, 31, 33, 35, 39, 40, 41, 43, 47, 48, 50, 51, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67} {
		if _, yes := attrs[k]; !yes {
			return r, afShape
		}
	}
	for k, v := range attrs {
		switch k {
		case 3, 4, 18, 20, 26: // identity/framing/AF checked below and by phase consumer
		case 6: // Non-authorizing scheduler label, policy-bounded, not safe-UP proof.
			q, e := attrText(attrs, 6, true)
			if e != nil || len(q) > 32 {
				return r, afShape
			}
			for _, c := range q {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
					return r, afShape
				}
			}
		case 7:
			if len(v) != 96 {
				return r, afShape
			}
		case 23:
			if len(v) != 200 {
				return r, afShape
			}
		case 14: // Policy permits zero struct form28 or32, not an ABI assertion.
			if len(v) != 28 && len(v) != 32 {
				return r, afShape
			}
			if !bytes.Equal(v, make([]byte, len(v))) {
				return r, afPhase
			}
		case 16:
			if len(v) != 1 {
				return r, afShape
			} // shape only; no guessed enum/allocator literal
		case 17, 39, 67:
			if len(v) != 1 || v[0] != 0 {
				return r, afPhase
			}
		case 33:
			if len(v) != 1 || v[0] != 1 {
				return r, afPhase
			} // actual creation excludes NO_CARRIER
		case 13, 27, 30, 31, 32, 35, 40, 41, 47, 48, 50, 51, 58, 59, 60, 61, 63, 64:
			if len(v) != 4 {
				return r, afShape
			}
			x := binary.NativeEndian.Uint32(v)
			switch k {
			case 13:
				if x != 500 {
					return r, afPhase
				}
			case 27, 30, 61:
				if x != 0 {
					return r, afPhase
				}
			case 31, 32:
				if x != 1 {
					return r, afPhase
				}
			case 50:
				if x != 68 {
					return r, afPhase
				}
			case 51:
				if x != 65535 {
					return r, afPhase
				}
			case 40, 41, 58, 59, 60, 63, 64: // width-only metadata, not allocator/offload proof
			}
		case 43:
			a, e := decodeAttributes(v)
			if e != nil || len(a) != 1 || len(a[2]) != 1 || a[2][0] != 0 {
				return r, afPhase
			}
		case 62, 65:
			if len(v) != 0 {
				return r, afPhase
			}
		case 66: // bounded accepted uint form, not helper/architecture theorem
			if len(v) != 4 && len(v) != 8 {
				return r, afShape
			}
			if !bytes.Equal(v, make([]byte, len(v))) {
				return r, afPhase
			}
		default:
			return r, afShape // topology/program/control unknowns not harmless metadata
		}
	}
	if up {
		if len(attrs[16]) != 1 || (attrs[16][0] != 0 && attrs[16][0] != 6) || len(attrs[17]) != 1 || attrs[17][0] != 0 {
			return r, afPhase
		}
		q, e := attrText(attrs, 6, true)
		if e != nil || q != "pfifo_fast" {
			return r, afPolicy
		}
	}
	if binary.NativeEndian.Uint32(attrs[35]) != binary.NativeEndian.Uint32(attrs[47])+binary.NativeEndian.Uint32(attrs[48]) {
		return r, afShape
	}
	if _, e = parseTUNNested(r); e != nil {
		return r, e
	}
	info, _ := decodeAttributes(attrs[18])
	data, e := decodeAttributes(info[2])
	if e != nil || len(data) != 5 {
		return r, afShape
	} // no OWNER/GROUP/extra queue
	if _, e = freshDisableAF(attrs[26]); e != nil {
		return r, e
	}
	return r, nil
}
func validateSetupOuter(l LinkRecord) error {
	if l.setupOuter == nil {
		return afShape
	}
	parsed, e := parseSetupOuterPhase(l.setupOuter.body, l.Flags&1 != 0)
	if e != nil {
		return e
	}
	l.setupOuter = nil
	if !reflect.DeepEqual(l, parsed) {
		return afShape
	}
	return nil
}

// Selection-only scan: validates framing and exactly one canonical IFNAME,
// retaining no authority from other fields. The selected path then runs the
// complete owned profile, or the unchanged generic foreign decoder. In
// particular no hardware-class or generic owned PAD decode is used to select.
func setupSelectionName(body []byte) (string, error) {
	if len(body) < 16 {
		return "", afShape
	}
	var name string
	found := false
	for raw := body[16:]; len(raw) > 0; {
		if len(raw) < 4 {
			return "", afShape
		}
		n := int(binary.NativeEndian.Uint16(raw))
		typ := binary.NativeEndian.Uint16(raw[2:])
		aligned := (n + 3) &^ 3
		if n < 4 || n > len(raw) || aligned > len(raw) {
			return "", afShape
		}
		for _, v := range raw[n:aligned] {
			if v != 0 {
				return "", afShape
			}
		}
		if typ&0x3fff == 3 {
			if typ != 3 || found {
				return "", afShape
			}
			found = true
			var e error
			name, e = attrText(map[uint16][]byte{3: raw[4:n]}, 3, true)
			if e != nil {
				return "", afShape
			}
		}
		raw = raw[aligned:]
	}
	if !found {
		return "", afShape
	}
	return name, nil
}

// Dump-only supported encoding policy, shared by baseline/foreign dump records.
// This is NOT helper/config-specific alignment proof. Only zero-payload plain
// PAD immediately before STATS64 or MAP is supported. Named singleton/flag
// validation still happens in the strict dump decoder after this scan.
func dumpAttrsWithoutPAD(raw []byte) (out []byte, hadPAD bool, err error) {
	// Receive-size limits belong to the existing transport, not a new
	// stricter foreign-link selector cap.
	pending := false
	for len(raw) > 0 {
		if len(raw) < 4 {
			return nil, false, afShape
		}
		n := int(binary.NativeEndian.Uint16(raw))
		typ := binary.NativeEndian.Uint16(raw[2:])
		aligned := (n + 3) &^ 3
		if n < 4 || n > len(raw) || aligned > len(raw) {
			return nil, false, afShape
		}
		for _, v := range raw[n:aligned] {
			if v != 0 {
				return nil, false, afShape
			}
		}
		if typ == 42 {
			if n != 4 || pending {
				return nil, false, afShape
			}
			pending = true
			hadPAD = true
			raw = raw[aligned:]
			continue
		}
		if pending && typ != 23 && typ != 14 {
			return nil, false, afShape
		}
		pending = false
		out = append(out, raw[:aligned]...)
		raw = raw[aligned:]
	}
	if pending {
		return nil, false, afShape
	}
	return out, hadPAD, nil
}
