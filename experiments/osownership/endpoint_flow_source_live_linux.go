//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

// Source-only final handshake step on the already-verified inherited lane.
// A decoded FLOW_START alone cannot construct this channel owner or change an
// original transaction. No original packet operation is permitted before it.
func (a *residentEndpointAdmission) acceptFlowSource(f endpointFrame) error {
	if a == nil || a.admitted == nil || a.started || f.kind != endpointFlowStart || f.direction != 1 || f.generation != a.admitted.Prepared.Generation {
		return afPhase
	}
	b, e := decodeEndpointFlow(f.payload)
	if e != nil || !b.matches(*a.admitted) {
		return errEndpointFrame
	}
	if e = a.checkMembershipSource(a.ctx, a.ns, a.start.binding()); e != nil {
		return e
	}
	a.started = true
	return nil
}
