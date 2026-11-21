//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"reflect"
)

type operation1PrecreationSource interface {
	observePrecreation(context.Context) (*precreationObservation, error)
}
type precreationObservation struct {
	owner                *operation1Controller
	baseline             *CapturedBaseline
	pass                 *afReadPass
	raw                  RawInventory
	sys                  SysctlSnapshot
	generation, sequence uint64
	used                 bool
	phase                string
	identity             adapterIdentity
	binding              Binding
}

func consumePrecreation(c *operation1Controller, o *precreationObservation) (Inventory, error) {
	if c == nil || o == nil || o.used {
		return Inventory{}, ErrConflict
	}
	o.used = true
	if c.tx == nil || c.adapter == nil || c.adapter.session == nil || c.attempt != nil || c.tx.stopStarted || c.tx.fdCloseAttempted || c.tx.Ready || c.adapter.session.canonical != nil || c.adapter.session.createAttempted || o.owner != c || o.identity != c.adapter.session.identity || o.binding != c.tx.plan.Binding || o.phase != "precreation" || o.baseline != c.adapter.session.baseline || o.sequence == 0 || o.sequence != c.precreationSequence || o.generation != c.precreationGeneration || o.generation != 1 || o.pass == nil || o.pass.sourceOwner == nil || o.pass.sourceOwner.controller != c {
		return Inventory{}, afPhase
	}
	if _, e := o.pass.requestClaim(); e != nil {
		return Inventory{}, e
	}
	if !exactSetupPostMatches(RawInventory{Links: o.pass.links}, RawInventory{Links: o.raw.Links}) || !reflect.DeepEqual(o.sys, o.pass.sys) || o.pass.effective == nil {
		return Inventory{}, afPolicy
	}
	observed, e := parseEffectiveV4(o.sys)
	if e != nil || !reflect.DeepEqual(observed, o.pass.effective) {
		return Inventory{}, afPolicy
	}
	if !originalBaselineMatches(c.tx.plan, o.baseline, o.raw, o.sys) {
		return Inventory{}, ErrConflict
	}
	// NO owned TUN exists. Compare only pinned copies for immutable baseline
	// counters, then use existing baseline policy Compose. No owned AF26 waiver.
	normalized, e := comparableSetupRaw(o.raw)
	if e != nil {
		return Inventory{}, e
	}
	baseRaw, e := comparableSetupRaw(o.baseline.raw)
	if e != nil {
		return Inventory{}, e
	}
	baseline := *o.baseline
	baseline.raw = baseRaw
	i, e := baseline.Compose(normalized, o.sys, nil, false)
	if e == nil {
		c.precreationAdmitted = true
	}
	return i, e
}
