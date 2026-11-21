//go:build mnet_enrollment_development

package osownership

import "encoding/binary"

// Exact1280 stays applicable: no below-minimum unregister prediction allowed.
// This narrow profile excludes routes whose PMTU/notifier work needs expansion.
func predictOwnedMTU6(l *LinkRecord, raw RawInventory) error {
	if l == nil || MTU != 1280 || l.Flags&1 != 0 {
		return afPhase
	}
	for _, a := range raw.Addresses {
		if a.Index == l.Index {
			return afPhase
		}
	}
	if len(raw.Routes) != 0 {
		return afPhase
	}
	families, e := decodeAttributes(l.Raw[26])
	if e != nil {
		return e
	}
	body, ok := families[10]
	if !ok {
		return ownedDisableDiffIncomplete
	}
	a6, e := freshDisableAF(l.Raw[26])
	if e != nil {
		return e
	}
	if len(a6[1]) != 4 || binary.NativeEndian.Uint32(a6[1]) != 0 || len(a6[2]) != 240 || len(a6[5]) != 16 {
		return afShape
	}
	// Fresh-DOWN profile: no RA MTU and no IF_READY/RS/RA flags.
	if _, ok := a6[9]; ok {
		return afPhase
	}
	binary.NativeEndian.PutUint32(a6[2][8:], uint32(MTU))
	families[10], e = replaceAttrValues(body, a6)
	if e != nil {
		return e
	}
	l.Raw[26], e = replaceAttrValues(l.Raw[26], families)
	return e
}

// Predicts the supported fresh-DOWN IPv6 disable poststate, not arbitrary
// addrconf_ifdown. Internal uncached/neighbor state is outside raw dump coverage.
// Normal trusted-admin/no-concurrent-configuration contract applies.
func predictFreshDownDisable(raw RawInventory, sys SysctlSnapshot, index int) (RawInventory, SysctlSnapshot, error) {
	out := cloneRawExact(raw)
	next := cloneSysExact(sys)
	if index <= 0 || len(raw.Routes) != 0 {
		return RawInventory{}, nil, afPhase
	}
	found := false
	for n := range out.Links {
		l := &out.Links[n]
		if l.Index != index {
			continue
		}
		if found || l.Hardware != 65534 || l.Flags & ^uint32(0x40|0x10000) != 0x1090 || l.MTU != 1280 {
			return RawInventory{}, nil, afPhase
		}
		// Pinned rtnl_link_info_fill emits INFO_DATA with nest_start_noflag.
		// Exact unflagged containers still require the complete TUN child schema.
		if _, e := parseTUNNested(*l); e != nil {
			return RawInventory{}, nil, e
		}
		for _, key := range []uint16{5, 10} {
			if v, ok := l.Raw[key]; ok && (len(v) != 4 || binary.NativeEndian.Uint32(v) != 0) {
				return RawInventory{}, nil, afPhase
			}
		}
		if mtu, e := attrUint(l.Raw, 4, true); e != nil || mtu != 1280 {
			return RawInventory{}, nil, afShape
		}
		found = true
		for _, a := range raw.Addresses {
			if a.Index == index {
				return RawInventory{}, nil, afPhase
			}
		}
		families, e := decodeAttributes(l.Raw[26])
		if e != nil {
			return RawInventory{}, nil, e
		}
		body, ok := families[10]
		if !ok {
			return RawInventory{}, nil, ownedDisableDiffIncomplete
		}
		a6, e := freshDisableAF(l.Raw[26])
		if e != nil {
			return RawInventory{}, nil, e
		}
		if len(a6[1]) != 4 || binary.NativeEndian.Uint32(a6[1]) != 0 || len(a6[2]) != 240 || len(a6[5]) != 16 {
			return RawInventory{}, nil, afShape
		}
		if _, ok := a6[9]; ok {
			return RawInventory{}, nil, afPhase
		}
		conf := a6[2]
		old := binary.NativeEndian.Uint32(conf[26*4:])
		if old > 1 || binary.NativeEndian.Uint32(conf[8:]) != 1280 {
			return RawInventory{}, nil, afPolicy
		}
		path := "/proc/sys/net/ipv6/conf/" + l.Name + "/disable_ipv6"
		value, ok := sys[path]
		if !ok || (old == 0 && value != "0") || (old == 1 && value != "1") {
			return RawInventory{}, nil, afPolicy
		}
		binary.NativeEndian.PutUint32(conf[26*4:], 1)
		next[path] = "1"
		// tstamp changes only for0->1; comparison uses existing pinned bytes4..7
		// mask. All other cacheinfo/flags/mode/secret-omitted-slot remain exact.
		families[10], e = replaceAttrValues(body, a6)
		if e != nil {
			return RawInventory{}, nil, e
		}
		l.Raw[26], e = replaceAttrValues(l.Raw[26], families)
		if e != nil {
			return RawInventory{}, nil, e
		}
	}
	if !found {
		return RawInventory{}, nil, afPhase
	}
	return out, next, nil
}

func freshDisableAF(af []byte) (map[uint16][]byte, error) {
	f, e := decodeAttributes(af)
	if e != nil || len(f) != 2 {
		return nil, afShape
	}
	v4, e := decodeAttributes(f[2])
	if e != nil || len(v4) != 1 || len(v4[1]) != 132 {
		return nil, afShape
	}
	a, e := decodeAttributes(f[10])
	if e != nil || len(a) != 7 {
		return nil, afShape
	}
	for k, n := range map[uint16]int{1: 4, 2: 240, 3: 304, 5: 16, 6: 56, 7: 16, 8: 1} {
		if len(a[k]) != n {
			return nil, afShape
		}
	}
	if binary.NativeEndian.Uint64(a[3]) != 38 || binary.NativeEndian.Uint64(a[6]) != 7 || binary.NativeEndian.Uint32(a[2][36*4:]) != 0 {
		return nil, afShape
	}
	if a[8][0] > 3 || uint32(a[8][0]) != binary.NativeEndian.Uint32(a[2][47*4:]) {
		return nil, afShape
	}
	return a, nil
}
