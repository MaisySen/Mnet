//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
	"strconv"
)

// UNUSED PROPOSAL, not wired into Compose or a live reader. Exact Linux v6.12
// skip-stats dump profile only. Unknown versions/shapes refuse, no truncation or
// normalization. This validates framing + named policy witnesses, not every
// devconf setting's semantics or the running kernel's provenance.
func validateOwnedAFSpecV612Proposal(l LinkRecord, s SysctlSnapshot) error {
	raw, ok := l.Raw[26]
	if !ok {
		return ErrConflict
	}
	families, e := decodeAttributes(raw)
	if e != nil || len(families) != 2 {
		return ErrConflict
	}
	v4, yes := families[2]
	if !yes {
		return ErrConflict
	}
	v6, yes := families[10]
	if !yes {
		return ErrConflict
	}
	a4, e := decodeAttributes(v4)
	if e != nil || len(a4) != 1 {
		return ErrConflict
	}
	conf4, yes := a4[1]
	if !yes || len(conf4) != 33*4 {
		return ErrConflict
	}
	// IPV4_DEVCONF_FORWARDING starts at 1, transmitted cnf.data[0].
	if binary.NativeEndian.Uint32(conf4[:4]) != 0 || s["/proc/sys/net/ipv4/conf/"+l.Name+"/forwarding"] != "0" {
		return ErrConflict
	}
	a6, e := decodeAttributes(v6)
	if e != nil || len(a6) != 3 {
		return ErrConflict
	}
	flags, yes := a6[1]
	if !yes || len(flags) != 4 || binary.NativeEndian.Uint32(flags) != 0 {
		return ErrConflict
	}
	conf6, yes := a6[2]
	if !yes || len(conf6) != 59*4 {
		return ErrConflict
	}
	cache, yes := a6[5]
	if !yes || len(cache) != 16 {
		return ErrConflict
	}
	// ipv6_store_devconf emits the exact indexed s32 vector. Named positions
	// are pinned from v6.12 ipv6.h, not guessed from proc-file iteration order.
	get := func(index int) uint32 { return binary.NativeEndian.Uint32(conf6[index*4 : index*4+4]) }
	prefix := "/proc/sys/net/ipv6/conf/" + l.Name + "/"
	if get(0) != 0 || get(26) != 1 || get(47) > 3 || s[prefix+"forwarding"] != "0" || s[prefix+"disable_ipv6"] != "1" || s[prefix+"addr_gen_mode"] != strconv.FormatUint(uint64(get(47)), 10) || s[ipv6DirPrefix+l.Name] != "present" {
		return ErrConflict
	}
	// stable_secret's devconf slot is intentionally unavailable in this profile;
	// require the documented zero fill rather than treating arbitrary bytes as
	// a secret or reading it. Not a secret retrieval path.
	if !bytes.Equal(conf6[36*4:37*4], []byte{0, 0, 0, 0}) {
		return ErrConflict
	}
	return nil
}
