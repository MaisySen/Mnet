//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Rule fields follow fib_rule_hdr/FRA_* from linux/fib_rules.h. No selector,
// suppress filter or unknown attr may be dropped in default-rule validation.
type RuleRecord struct {
	Family, DstBits, SrcBits, TOS, Table, Reserved1, Reserved2, Action byte
	Flags                                                              uint32
	Raw                                                                map[uint16][]byte
}

func DecodeRule(b []byte) (RuleRecord, error) {
	var r RuleRecord
	if len(b) < 12 {
		return r, errors.New("short fib_rule_hdr")
	}
	r = RuleRecord{Family: b[0], DstBits: b[1], SrcBits: b[2], TOS: b[3], Table: b[4], Reserved1: b[5], Reserved2: b[6], Action: b[7], Flags: binary.NativeEndian.Uint32(b[8:])}
	a, e := decodeAttributes(b[12:])
	r.Raw = a
	return r, e
}
func ValidateDefaultRules(bs [][]byte) error {
	want := map[[3]uint32]bool{{2, 255, 0}: true, {2, 254, 32766}: true, {2, 253, 32767}: true, {10, 255, 0}: true, {10, 254, 32766}: true, {128, 253, 32767}: true, {129, 254, 32767}: true}
	if len(bs) != len(want) {
		return errors.New("nondefault rule count")
	}
	for _, b := range bs {
		r, e := DecodeRule(b)
		if e != nil {
			return e
		}
		if r.DstBits != 0 || r.SrcBits != 0 || r.TOS != 0 || r.Reserved1 != 0 || r.Reserved2 != 0 || r.Action != 1 || r.Flags != 0 {
			return errors.New("nondefault rule header")
		}
		for k := range r.Raw {
			if k != 6 && k != 14 && k != 15 && k != 21 {
				return fmt.Errorf("nondefault rule attr %d", k)
			}
		}
		table, e := attrUint(r.Raw, 15, true)
		if e != nil || table != uint32(r.Table) {
			return errors.New("rule table mismatch")
		}
		suppress, e := attrUint(r.Raw, 14, true)
		if e != nil || suppress != 0xffffffff {
			return errors.New("rule suppression changed")
		}
		proto := r.Raw[21]
		if len(proto) != 1 || proto[0] != 2 {
			return errors.New("nonkernel rule protocol")
		}
		priority, e := attrUint(r.Raw, 6, false)
		if e != nil {
			return e
		}
		key := [3]uint32{uint32(r.Family), table, priority}
		if !want[key] {
			return errors.New("foreign or duplicate rule")
		}
		delete(want, key)
	}
	if len(want) != 0 {
		return errors.New("missing default rules")
	}
	return nil
}
