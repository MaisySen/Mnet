//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"time"
)

// RawInventory preserves records/attrs and rules verbatim. It is NOT converted
// to policy-safe Inventory until rule/sysctl and kernel-origin semantics are
// reviewed. Multiple RTNL dumps are not atomic: require two complete sets equal
// under the pinned comparison policy in the exclusively owned namespace. Interrupted or
// malformed dumps restart the WHOLE inventory, at most2 attempts/250ms.
type RawInventory struct {
	Links     []LinkRecord
	Addresses []AddressRecord
	Routes    []RouteRecord
	Rules     [][]byte
}
type DumpSource interface {
	Request(context.Context, uint16, []byte, bool) ([][]byte, error)
}

func ReadStableInventory(ctx context.Context, d DumpSource, pinnedCheck func() error) (RawInventory, error) {
	var zero RawInventory
	if d == nil || pinnedCheck == nil {
		return zero, ErrConflict
	}
	bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		a, e := readOne(bounded, d, pinnedCheck)
		if e != nil {
			last = e
			continue
		}
		b, e := readOne(bounded, d, pinnedCheck)
		if e != nil {
			last = e
			continue
		}
		if exactSetupPostMatches(a, b) {
			// Return ORIGINAL unmasked second observation, not comparison copies.
			return b, nil
		}
		last = errors.New("inventory changed between complete dumps")
	}
	return zero, last
}

type setupLinkDumpDecoder interface {
	decodeOwnedSetupLink([]byte) (LinkRecord, error)
}

func readOne(ctx context.Context, d DumpSource, pinned func() error) (RawInventory, error) {
	var i RawInventory
	for _, kind := range []uint16{18, 22, 26, 34} {
		if e := ctx.Err(); e != nil {
			return i, e
		}
		if e := pinned(); e != nil {
			return i, e
		}
		header, e := DumpRequestHeader(kind)
		if e != nil {
			return i, e
		}
		ms, e := d.Request(ctx, kind, header, true)
		if e != nil {
			return i, e
		}
		if e := pinned(); e != nil {
			return i, e
		}
		if len(ms) > 4096 {
			return i, errors.New("inventory record cap")
		}
		for _, b := range ms {
			switch kind {
			case 18:
				var r LinkRecord
				var e error
				if scoped, ok := d.(setupLinkDumpDecoder); ok {
					r, e = scoped.decodeOwnedSetupLink(b)
				} else {
					r, e = DecodeLink(b)
				}
				if e != nil {
					return i, e
				}
				i.Links = append(i.Links, r)
			case 22:
				r, e := DecodeAddress(b)
				if e != nil {
					return i, e
				}
				i.Addresses = append(i.Addresses, r)
			case 26:
				r, e := DecodeRoute(b)
				if e != nil {
					return i, e
				}
				i.Routes = append(i.Routes, r)
			case 34:
				if len(b) < 12 {
					return i, errors.New("short fib rule")
				}
				if _, e := decodeAttributes(b[12:]); e != nil {
					return i, e
				}
				i.Rules = append(i.Rules, append([]byte(nil), b...))
			}
		}
	}
	if e := pinned(); e != nil {
		return i, e
	}
	return i, nil
}
