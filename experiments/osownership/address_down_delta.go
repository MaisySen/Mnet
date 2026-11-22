//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
	"reflect"
)

// Exact supported zero-offload local /32 emitter profile. v6.17 fib_magic ->
// fib_table_insert -> fib_create_info HOST-scope branch does not require UP and
// does not set LINKDOWN. fib_dump_info single-nexthop overwrites header flags
// from fib_nexthop_info. Reject any observed offload/trap rather than mask it.
// No metrics/priority/gateway/nh-id/lwt attributes arise from this constructor.
func validateAddressLocalRoute(h *Handle, r RouteRecord) error {
	if h == nil || !h.Address.Prefix.IsValid() || !h.Address.Prefix.Addr().Is4() || h.Link.Index <= 0 || r.Family != 2 || r.DestBits != 32 || r.SourceBits != 0 || r.TOS != 0 || r.Table != 255 || r.TableID != 255 || r.Protocol != 2 || r.Scope != 254 || r.Type != 2 || r.Flags != 0 || r.Dst != h.Address.Prefix.Addr() || r.PreferredSource != r.Dst || r.Source.IsValid() || r.Gateway.IsValid() || r.OIF != uint32(h.Link.Index) || r.IIF != 0 || r.Priority != 0 || len(r.Raw) != 4 {
		return ErrConflict
	}
	for _, k := range []uint16{1, 4, 7, 15} {
		if _, ok := r.Raw[k]; !ok {
			return ErrConflict
		}
	}
	if !bytes.Equal(r.Raw[1], ipBytes(r.Dst)) || !bytes.Equal(r.Raw[7], ipBytes(r.PreferredSource)) {
		return ErrConflict
	}
	for k, want := range map[uint16]uint32{4: r.OIF, 15: r.TableID} {
		v, e := attrUint(r.Raw, k, true)
		if e != nil || v != want {
			return ErrConflict
		}
	}
	return nil
}

// Functional combined readback validation, not a mutation permit/result. The
// caller must independently establish custody, current generation, exact plan,
// ACK and supported source environment before it may use the returned evidence.
// Unlike tiny route/address fixtures, this compares the whole remaining dump,
// including every route table/rule/link and AF IPv4 own-conf array, plus all
// supplied sysctl observations. It does not claim internal notifier coverage.
func captureAddressDownDelta(h *Handle, before RawInventory, beforeSys SysctlSnapshot, after RawInventory, afterSys SysctlSnapshot) (*ownedAddressDumpCandidate, error) {
	if h == nil || len(before.Addresses) != 0 || len(before.Routes) != 0 || len(after.Addresses) != 1 || len(after.Routes) != 1 || !reflect.DeepEqual(beforeSys, afterSys) {
		return nil, ErrConflict
	}
	if e := requireAddressDownLink(h, before, beforeSys); e != nil {
		return nil, e
	}
	if e := requireAddressDownLink(h, after, afterSys); e != nil {
		return nil, e
	}
	address, e := captureOwnedAddressRecordCandidate(h, after.Addresses[0])
	if e != nil {
		return nil, e
	}
	if e = validateAddressLocalRoute(h, after.Routes[0]); e != nil {
		return nil, e
	}
	rest := cloneRawExact(after)
	// Preserve nil versus empty baseline slice identity. Only these two observed
	// additions may be subtracted. Nothing filters routes by owned OIF alone.
	rest.Addresses = before.Addresses
	rest.Routes = before.Routes
	if !exactSetupPostMatches(rest, before) {
		return nil, ErrConflict
	}
	return address, nil
}

func requireAddressDownLink(h *Handle, raw RawInventory, sys SysctlSnapshot) error {
	found := false
	for _, l := range raw.Links {
		if l.Index != h.Link.Index {
			continue
		}
		if found || l.Name != h.Link.Name || l.Alias != h.Link.Alias || l.MTU != h.Link.MTU || l.Hardware != 65534 || l.Flags != 0x1090 {
			return ErrConflict
		}
		found = true
		a, e := freshDisableAF(l.Raw[26])
		if e != nil {
			return e
		}
		if binary.NativeEndian.Uint32(a[1]) != 0 || binary.NativeEndian.Uint32(a[2][26*4:]) != 1 || sys["/proc/sys/net/ipv6/conf/"+l.Name+"/disable_ipv6"] != "1" {
			return ErrConflict
		}
	}
	if !found {
		return ErrConflict
	}
	return nil
}
