//go:build mnet_enrollment_development

package osownership

import "context"

// Deliberately inert typed transition scaffold. No constructor can mint a
// successful result yet. Pure address schema evidence is not full poststate:
// exact route tables/rules, FIB insert/nexthop/emitter, all notifier effects,
// device own-conf readback and disable ancestry at NEW generation remain owed.
// Address allocation marks ipv4_devconf_setall and
// neigh_parms_data_state_setall: future defaults cease propagating. Those are
// kernel bookkeeping effects, not permission for us to write device conf.
// A later result must retain lifetime/ancestry without weakening D.current().
type postAddressDownResult struct {
	disabledAncestry *postDisableResult
	lifetime         *disableResidentLifetime
	ledger           *heldPacketLedger
	identity         heldPacketIdentity
	address          *ownedAddressDumpCandidate
}

// Never use a phase integer or candidate parser success to authorize address
// send. Missing route source and full effect validation are independent holds.
func refuseAddressDownSend(context.Context, *postDisableResult, *ownedAddressDumpCandidate) (*postAddressDownResult, error) {
	return nil, afPhase
}

// No Inventory/Ready/supplier conversion. Even a package-internal literal with
// plausible fields cannot advance step4 while the full transition is held.
func (*postAddressDownResult) admitStep4(context.Context) error { return afPhase }
func (*postAddressDownResult) current() bool                    { return false }
