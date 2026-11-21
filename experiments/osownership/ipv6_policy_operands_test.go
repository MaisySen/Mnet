//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"sort"
	"strings"
	"testing"
)

func ipv6PolicyOperandFixture() SysctlSnapshot {
	s := SysctlSnapshot{}
	for _, p := range ipv6PolicyPaths() {
		s[p] = "0"
		if strings.HasSuffix(p, "/ra_defrtr_metric") {
			s[p] = "1"
		}
	}
	return s
}
func TestIPv6OperandAllowlistNeverContainsSecret(t *testing.T) {
	paths := ipv6PolicyPaths()
	if len(paths) != 124 {
		t.Fatalf("nonsecret operand paths=%d", len(paths))
	}
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] || !permittedIPv6OperandPath(p) || strings.Contains(p, "secret") {
			t.Fatal("duplicate or forbidden path", p)
		}
		seen[p] = true
	}
	if ipv6PolicyLeaves[36] != "" || permittedIPv6OperandPath("ipv6/conf/default/stable_secret") {
		t.Fatal("secret slot can be probed")
	}
}
func TestIPv6OperandMissingConfigNeverMeansZero(t *testing.T) {
	s := ipv6PolicyOperandFixture()
	delete(s, "/proc/sys/net/ipv6/conf/all/seg6_require_hmac")
	if _, e := parseIPv6PolicyOperands(s); e == nil {
		t.Fatal("missing config accepted")
	}
}
func TestIPv6OperandSignedUnsignedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		leaf, value string
		ok          bool
	}{
		{"use_tempaddr", "-1", true}, {"ioam6_id_wide", "4294967295", true}, {"ioam6_id_wide", "-1", false}, {"forwarding", "4294967295", false}, {"ioam6_id_wide", "4294967296", false}, {"use_tempaddr", "-2147483649", false}, {"accept_dad", "01", false}, {"accept_dad", " 1", false},
	} {
		s := ipv6PolicyOperandFixture()
		s["/proc/sys/net/ipv6/conf/all/"+tc.leaf] = tc.value
		_, e := parseIPv6PolicyOperands(s)
		if (e == nil) != tc.ok {
			t.Fatalf("%+v: %v", tc, e)
		}
	}
}
func TestIPv6ConsumerFormulaIsNotSafety(t *testing.T) {
	l, _ := fullAFSynthetic()
	s := ipv6PolicyOperandFixture()
	s["/proc/sys/net/ipv6/conf/all/forwarding"] = "1"
	s["/proc/sys/net/ipv6/conf/all/seg6_enabled"] = "0"
	s["/proc/sys/net/ipv6/conf/all/accept_source_route"] = "-1"
	o, e := parseIPv6PolicyOperands(s)
	if e != nil {
		t.Fatal(e)
	}
	f, e := decodeAttributes(l.Raw[26])
	if e != nil {
		t.Fatal(e)
	}
	attrs, e := decodeAttributes(f[10])
	if e != nil {
		t.Fatal(e)
	}
	for i, v := range map[int]uint32{0: 1, 3: 1, 4: 1, 24: 0, 44: 1, 53: 1, 59: 0} {
		binary.NativeEndian.PutUint32(attrs[2][4*i:], v)
	}
	// decodeAttributes returns copied sub-slices: deliberately construct only AF
	// numerical operand changes, no source-lineage or operational claim.
	f[10] = encodeIPv6PolicyTestAttributes(attrs)
	l.Raw[26] = encodeIPv6PolicyTestAttributes(f)
	r, e := assessIPv6ConsumerOperands(l, o)
	if e != nil {
		t.Fatal(e)
	}
	if r[0].Effective != 1 || r[3].Effective != 0 || r[4].Effective != 0 || r[24].Effective != -1 || r[44].Effective != 0 || r[53].Effective != 1 {
		t.Fatalf("wrong per-consumer formulas: %+v", r)
	}
	for _, w := range r {
		if w.Resolved {
			t.Fatal("formula became safety", w.Index)
		}
	}
	if r[36].LocalObserved || r[36].Computed {
		t.Fatal("secret omission became observation")
	}
}
func TestIPv6ConsumerMetadataUnitsAndConfig(t *testing.T) {
	o, e := parseIPv6PolicyOperands(ipv6PolicyOperandFixture())
	if e != nil {
		t.Fatal(e)
	}
	r := ipv6ConsumerRules(o)
	if len(r) != 60 || r[8].ProcUnit == r[8].AFUnit || !strings.Contains(r[45].Config, "HMAC") || r[36].Leaf != "" {
		t.Fatal("metadata gap")
	}
}

func encodeIPv6PolicyTestAttributes(m map[uint16][]byte) []byte {
	keys := []int{}
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	var out []byte
	for _, k := range keys {
		out = append(out, attr(uint16(k), m[uint16(k)])...)
	}
	return out
}
