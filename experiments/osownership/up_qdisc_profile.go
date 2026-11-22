//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"sort"
)

// First supported actual graph profile, not a universal default claim. Exact
// v6.17 sch_generic pfifo_fast_ops has no cl_ops/filter blocks. Full RTM_GETQDISC
// dump covers root/hash/ingress on every target device. Retain foreign bodies;
// no block/ingress/extra owned graph or invisible option is silently omitted.
type upQdiscObservation struct {
	bodies [][]byte
}
type upQdiscProfile struct {
	foreign [][]byte
	owned   []byte
}

func parseUpQdisc(o upQdiscObservation, index int) (*upQdiscProfile, error) {
	if index <= 0 || len(o.bodies) > 4096 {
		return nil, afPolicy
	}
	out := &upQdiscProfile{}
	for _, b := range o.bodies {
		if len(b) < 20 || len(b) > 16384 {
			return nil, afShape
		}
		if int(int32(binary.NativeEndian.Uint32(b[4:]))) != index {
			out.foreign = append(out.foreign, append([]byte(nil), b...))
			continue
		}
		if out.owned != nil || !bytes.Equal(b[:4], make([]byte, 4)) || binary.NativeEndian.Uint32(b[8:]) != 0 || binary.NativeEndian.Uint32(b[12:]) != 0xffffffff || binary.NativeEndian.Uint32(b[16:]) == 0 {
			return nil, afPhase
		}
		a, e := tcStatsAttrs(b[20:], 9)
		if e != nil {
			return nil, e
		}
		if len(a) != 5 {
			return nil, afShape
		}
		kind, e := attrText(a, 1, true)
		if e != nil || kind != "pfifo_fast" || len(a[12]) != 1 || a[12][0] != 0 {
			return nil, afPolicy
		}
		want := append(u32(3), []byte{1, 2, 2, 2, 1, 2, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1}...)
		if !bytes.Equal(a[2], want) || len(a[3]) != 40 {
			return nil, afShape
		}
		stats, e := tcStatsAttrs(a[7], 6)
		if e != nil || len(stats) != 2 || len(stats[1]) != 16 || len(stats[3]) != 20 {
			return nil, afShape
		}
		// No supplier/application traffic yet: no queued packet/backlog. Throughput
		// counters must also be zero in this construction-only profile. No rate estimator,
		// app stats, 64bit packet overflow, hardware/block/stab extension supported.
		if !bytes.Equal(stats[1], make([]byte, 16)) || !bytes.Equal(stats[3], make([]byte, 20)) || !bytes.Equal(a[3], make([]byte, 40)) {
			return nil, afPhase
		}
		for k := range a {
			if k != 1 && k != 2 && k != 3 && k != 7 && k != 12 {
				return nil, afShape
			}
		}
		out.owned = append([]byte(nil), b...)
	}
	if out.owned == nil {
		return nil, afPhase
	}
	return out, nil
}

// gnet_stats uses unflagged containers and alignment PAD. Permit only empty
// PAD immediately before a stats field; no runs/trailing PAD or flags/duplicates.
func tcStatsAttrs(b []byte, pad uint16) (map[uint16][]byte, error) {
	var named []byte
	pending := false
	for len(b) > 0 {
		if len(b) < 4 {
			return nil, afShape
		}
		n := int(binary.NativeEndian.Uint16(b))
		k := binary.NativeEndian.Uint16(b[2:])
		aligned := (n + 3) &^ 3
		if n < 4 || aligned > len(b) {
			return nil, afShape
		}
		if k == pad {
			if n != 4 || pending {
				return nil, afShape
			}
			pending = true
		} else {
			if pending && ((pad == 9 && k != 7 && k != 3) || (pad == 6 && k != 1 && k != 3)) {
				return nil, afShape
			}
			named = append(named, b[:aligned]...)
			pending = false
		}
		b = b[aligned:]
	}
	if pending {
		return nil, afShape
	}
	return decodeAttributes(named)
}

// Construction-only profile: stable graph/options, no traffic, positive refcount.
// Independent observations may differ in transient positive refcounts. Foreign
// graph gets exactly the same schema, never an unknown-kind volatility waiver.
func constructionQdiscKeys(o upQdiscObservation) ([][]byte, error) {
	var out [][]byte
	if len(o.bodies) > 4096 {
		return nil, afShape
	}
	for _, b := range o.bodies {
		if len(b) < 20 {
			return nil, afShape
		}
		idx := int(int32(binary.NativeEndian.Uint32(b[4:])))
		if _, e := parseUpQdisc(upQdiscObservation{bodies: [][]byte{b}}, idx); e != nil {
			return nil, e
		}
		key := append([]byte(nil), b[:16]...)
		a, e := tcStatsAttrs(b[20:], 9)
		if e != nil {
			return nil, e
		}
		key = append(key, a[1]...)
		key = append(key, a[2]...)
		key = append(key, a[12]...)
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i], out[j]) < 0 })
	for i := 1; i < len(out); i++ {
		if bytes.Equal(out[i-1], out[i]) {
			return nil, afShape
		}
	}
	return out, nil
}
func sameConstructionQdisc(a, b upQdiscObservation) bool {
	x, e := constructionQdiscKeys(a)
	if e != nil {
		return false
	}
	y, e := constructionQdiscKeys(b)
	return e == nil && reflect.DeepEqual(x, y)
}
