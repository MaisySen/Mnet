//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
)

// Candidate schema only. Not used by validateOwnedAddress, source dispatch, or
// Inventory conversion. v6.17 devinet.c inet_fill_ifaddr/put_cacheinfo establish
// emission semantics. Exact v6.17 include/uapi/linux/if_addr.h now grounds
// numeric ABI/layout (official kernel.org archive, S74 provenance packet).
// This parser remains non-authorizing until full transition integration.
const (
	addressDumpAddress       uint16 = 1
	addressDumpLocal         uint16 = 2
	addressDumpLabel         uint16 = 3
	addressDumpCache         uint16 = 6
	addressDumpFlags         uint16 = 8
	addressDumpPermanent     uint32 = 0x80
	addressDumpNoPrefixRoute uint32 = 0x200
	// Held v6.17 include/net/addrconf.h INFINITY_LIFE_TIME and timeout helpers.
	addressDumpInfinity uint32 = 0xffffffff
)

type addressDumpCacheinfo struct {
	preferred, valid, created, updated uint32
}

// All four cache words are observed, not synthesized from wall time. Permanent
// lifetimes do not count down. No equality/order/nonzero condition on timestamps
// is justified: put_cacheinfo emits independently converted u32 observations.
type ownedAddressDumpCandidate struct {
	record AddressRecord
	label  string
	flags  uint32
	cache  addressDumpCacheinfo
}

func captureOwnedAddressDumpCandidate(h *Handle, body []byte) (*ownedAddressDumpCandidate, error) {
	if h == nil || !safeInterfaceName(h.Link.Name) || !h.Address.Prefix.IsValid() || !h.Address.Prefix.Addr().Is4() || h.Address.Prefix.Bits() != 32 || h.Address.Prefix.Addr().IsUnspecified() {
		return nil, ErrConflict
	}
	a, err := DecodeAddress(body)
	if err != nil {
		return nil, err
	}
	return captureOwnedAddressRecordCandidate(h, a)
}

func captureOwnedAddressRecordCandidate(h *Handle, a AddressRecord) (*ownedAddressDumpCandidate, error) {
	if h == nil || !safeInterfaceName(h.Link.Name) || !h.Address.Prefix.IsValid() || !h.Address.Prefix.Addr().Is4() || h.Address.Prefix.Bits() != 32 || h.Address.Prefix.Addr().IsUnspecified() {
		return nil, ErrConflict
	}
	a.Raw = cloneRawAttrs(a.Raw)
	if !a.Address.Is4() || !a.Local.Is4() || !bytes.Equal(a.Raw[addressDumpAddress], ipBytes(a.Address)) || !bytes.Equal(a.Raw[addressDumpLocal], ipBytes(a.Local)) {
		return nil, ErrConflict
	}
	if a.Index != h.Link.Index || a.Family != 2 || a.Prefix != 32 || a.Scope != 0 || a.Local != h.Address.Prefix.Addr() || a.Address != a.Local {
		return nil, ErrConflict
	}
	// Default request has nonzero LOCAL/ADDRESS and device-name LABEL. No
	// broadcast/proto/priority or target-netnsid override, hence exact presence.
	if len(a.Raw) != 5 {
		return nil, ErrConflict
	}
	for _, k := range []uint16{addressDumpAddress, addressDumpLocal, addressDumpLabel, addressDumpCache, addressDumpFlags} {
		if _, ok := a.Raw[k]; !ok {
			return nil, ErrConflict
		}
	}
	flags, err := attrUint(a.Raw, addressDumpFlags, true)
	if err != nil || flags != addressDumpPermanent|addressDumpNoPrefixRoute || a.Flags != byte(flags) {
		return nil, ErrConflict
	}
	label, err := attrText(a.Raw, addressDumpLabel, true)
	if err != nil || label != h.Link.Name {
		return nil, ErrConflict
	}
	raw := a.Raw[addressDumpCache]
	if len(raw) != 16 {
		return nil, ErrConflict
	}
	cache := addressDumpCacheinfo{binary.NativeEndian.Uint32(raw), binary.NativeEndian.Uint32(raw[4:]), binary.NativeEndian.Uint32(raw[8:]), binary.NativeEndian.Uint32(raw[12:])}
	if cache.preferred != addressDumpInfinity || cache.valid != addressDumpInfinity {
		return nil, ErrConflict
	}
	return &ownedAddressDumpCandidate{record: a, label: label, flags: flags, cache: cache}, nil
}

// Revalidate the complete later observation, then require exact attr/cache
// stability. No tolerance, cstamp=tstamp assumption, or cache masking. Attribute
// order is not identity, but duplicate/flagged attrs fail in DecodeAddress.
func (c *ownedAddressDumpCandidate) matches(h *Handle, body []byte) bool {
	if c == nil {
		return false
	}
	next, err := captureOwnedAddressDumpCandidate(h, body)
	if err != nil || c.record.Index != next.record.Index || c.record.Local != next.record.Local || c.label != next.label || c.flags != next.flags || c.cache != next.cache {
		return false
	}
	for k, v := range c.record.Raw {
		if !bytes.Equal(v, next.record.Raw[k]) {
			return false
		}
	}
	return true
}
