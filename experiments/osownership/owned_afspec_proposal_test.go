//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"testing"
)

// Helper-only synthetic schema oracle, NOT active Compose/readiness or actual
// kernel dump proof. Proposal is unused and its integration patch not applied.
func afspecProposalFixture() (LinkRecord, SysctlSnapshot) {
	name := "mno-proposal"
	c4 := make([]byte, 132)
	c6 := make([]byte, 236)
	binary.NativeEndian.PutUint32(c6[26*4:], 1)
	binary.NativeEndian.PutUint32(c6[47*4:], 1)
	v6 := append(attr(1, make([]byte, 4)), attr(2, c6)...)
	v6 = append(v6, attr(5, make([]byte, 16))...)
	l := LinkRecord{Name: name, Raw: map[uint16][]byte{26: append(attr(2, attr(1, c4)), attr(10, v6)...)}}
	p := "/proc/sys/net/ipv6/conf/" + name + "/"
	s := SysctlSnapshot{"/proc/sys/net/ipv4/conf/" + name + "/forwarding": "0", p + "forwarding": "0", p + "disable_ipv6": "1", p + "addr_gen_mode": "1", ipv6DirPrefix + name: "present"}
	return l, s
}
func TestUnusedAFSpecProposalRejectsShapeAndWitnessMutations(t *testing.T) {
	l, s := afspecProposalFixture()
	if e := validateOwnedAFSpecV612Proposal(l, s); e != nil {
		t.Fatal("synthetic helper framing", e)
	}
	for _, mode := range []string{"unknown-family", "duplicate-family", "empty-v4", "unknown-v6", "v4-length", "v6-length", "cache-length", "flags", "forwarding", "disable", "addrgen", "secret-slot", "sysctl", "directory", "padding", "nested-flag"} {
		t.Run(mode, func(t *testing.T) {
			l, s := afspecProposalFixture()
			families, _ := decodeAttributes(l.Raw[26])
			v4, _ := decodeAttributes(families[2])
			v6, _ := decodeAttributes(families[10])
			switch mode {
			case "unknown-family":
				l.Raw[26] = append(l.Raw[26], attr(77, nil)...)
			case "duplicate-family":
				l.Raw[26] = append(l.Raw[26], attr(2, families[2])...)
			case "empty-v4":
				families[2] = nil
			case "unknown-v6":
				families[10] = append(families[10], attr(77, nil)...)
			case "v4-length":
				v4[1] = v4[1][:128]
			case "v6-length":
				v6[2] = v6[2][:232]
			case "cache-length":
				v6[5] = v6[5][:12]
			case "flags":
				v6[1][0] = 1
			case "forwarding":
				v4[1][0] = 1
			case "disable":
				binary.NativeEndian.PutUint32(v6[2][26*4:], 0)
			case "addrgen":
				binary.NativeEndian.PutUint32(v6[2][47*4:], 4)
			case "secret-slot":
				v6[2][36*4] = 1
			case "sysctl":
				s["/proc/sys/net/ipv6/conf/"+l.Name+"/disable_ipv6"] = "0"
			case "directory":
				s[ipv6DirPrefix+l.Name] = "ENOENT"
			case "padding":
				l.Raw[26] = append(l.Raw[26], []byte{5, 0, 88, 0, 1, 1, 0, 0}...)
			case "nested-flag":
				binary.NativeEndian.PutUint16(l.Raw[26][2:4], 0x8002)
			}
			switch mode {
			case "v4-length", "forwarding":
				families[2] = attr(1, v4[1])
			case "v6-length", "cache-length", "flags", "disable", "addrgen", "secret-slot":
				families[10] = append(append(attr(1, v6[1]), attr(2, v6[2])...), attr(5, v6[5])...)
			}
			switch mode {
			case "empty-v4", "unknown-v6", "v4-length", "forwarding", "v6-length", "cache-length", "flags", "disable", "addrgen", "secret-slot":
				l.Raw[26] = append(attr(2, families[2]), attr(10, families[10])...)
			}
			if e := validateOwnedAFSpecV612Proposal(l, s); e == nil {
				t.Fatal("bad proposal shape accepted", mode)
			}
		})
	}
}
