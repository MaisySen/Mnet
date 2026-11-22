//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"net/netip"
	"os"
)

// Serializes only an already source-checked local descriptor. Returned bytes are
// not a receipt or channel authority. Parent still compares against its exact
// retained process lifetime and native session before any channel admission.
func (p *preparedEndpointObservation) controlBodySource(ctx context.Context) (endpointPreparedBody, error) {
	var zero endpointPreparedBody
	d, e := p.descriptorSource(ctx)
	if e != nil {
		return zero, e
	}
	own, e := netip.ParseAddr(d.OwnIP)
	if e != nil || !own.Is4() {
		return zero, ErrConflict
	}
	peer, e := netip.ParseAddr(d.PeerIP)
	if e != nil || !peer.Is4() {
		return zero, ErrConflict
	}
	a, e := endpointKey(d.OwnKey)
	if e != nil {
		return zero, e
	}
	b, e := endpointKey(d.PeerKey)
	if e != nil {
		return zero, e
	}
	pid := os.Getpid()
	ticks, e := readProcessStartTicks(pid)
	if e != nil {
		return zero, e
	}
	out := endpointPreparedBody{PID: uint32(pid), StartTicks: ticks, Namespace: d.Namespace, Generation: d.Generation, MTU: uint32(d.MTU), Own: own.As4(), Peer: peer.As4(), OwnKey: a, PeerKey: b, SelfUntilMS: d.SelfUntilMS, PeerUntilMS: d.PeerUntilMS}
	if _, e = out.encode(); e != nil {
		return zero, e
	}
	if e = p.local.resources.namespace.checkCurrent(); e != nil {
		return zero, e
	}
	return out, nil
}
