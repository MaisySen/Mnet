//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
)

// Concrete parent lifetime monitor after explicit native admission. It owns its
// pipe lane exclusively. No fabricated WG tun.Device/foreign callback can turn
// a child frame into authenticated plaintext here. Datagrams are refused until
// the actual exclusive parent WireGuard/config owner is implemented and bound.
// In the current tree child completion refuses before any such datagram exists.
func (p *endpointParentSource) serveSource(ctx context.Context) (ret error) {
	if p == nil || p.admission == nil {
		return afPhase
	}
	defer func() { ret = errors.Join(ret, p.stopSource(ctx)) }()
	for {
		if e := p.currentSource(ctx); e != nil {
			return e
		}
		f, ok, e := p.wire.readSource()
		if e != nil {
			return e
		}
		if !ok {
			if e = p.wire.waitSource(ctx, false); e != nil {
				return e
			}
			continue
		}
		switch f.kind {
		case endpointStopped, endpointStats:
			b, e := decodeEndpointStats(f.payload)
			if e != nil {
				endpointCount(&p.counts.FramingErrors)
				return e
			}
			p.lastStats = &b
			endpointCount(&p.counts.ControlRead)
			if !b.CleanupVerified || b.Reason == endpointReasonPolicyIncomplete {
				return ownedAFPolicyIncomplete
			}
			return nil
		case endpointDatagram:
			endpointCount(&p.counts.DatagramsDropped)
			return ownedAFPolicyIncomplete
		default:
			endpointCount(&p.counts.FramingErrors)
			return errEndpointFrame
		}
	}
}
