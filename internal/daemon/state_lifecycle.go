package daemon

import (
	"errors"
	"path/filepath"
)

// Requires d.mu. Faults end identity use and cancel current network work. The
// retained owner is released only by Close after workers/teardown finish.
func (d *Daemon) stateHealthLocked() error {
	if d.closed {
		return errors.New("node is closed")
	}
	if d.stateOwner == nil {
		return errors.New("native state owner absent")
	}
	if err := d.stateOwner.health(); err != nil {
		d.freezeStateLocked()
		return err
	}
	return nil
}
func (d *Daemon) freezeStateLocked() {
	if d.stateFaultHandled {
		return
	}
	d.stateFaultHandled = true
	d.stateOwner.fault = true
	d.lastErr = "native state unavailable; Close and reopen required"
	d.status = "state-unavailable"
	d.lastMap = nil
	d.cc = nil
	if d.cancel != nil {
		d.cancel()
	}
	// Never wait for lifecycle/worker teardown while holding d.mu.
	go d.Down()
}
func (d *Daemon) saveStateLocked() error { return d.saveNativeStateLocked(true) }
func (d *Daemon) saveNativeStateLocked(rollback bool) error {
	if err := d.stateHealthLocked(); err != nil {
		return err
	}
	if d.stateOwner == nil {
		return errors.New("native state owner absent")
	}
	// cfg mutation is not permission to redirect owned identity writes.
	actual, err := filepath.Abs(d.cfg.StateDir)
	if err != nil {
		return err
	}
	actual, err = filepath.EvalSymlinks(actual)
	if err != nil {
		return err
	}
	if actual != d.stateOwner.lease.Parent() {
		return ErrStateStale
	}
	if err := d.stateOwner.save(d.state); err != nil {
		if d.stateOwner.fault {
			d.freezeStateLocked()
		} else if rollback {
			*d.state = cloneState(d.stateOwner.saved)
		}
		return err
	}
	return nil
}
func (d *Daemon) ensureStateKeyLocked() error {
	if err := d.stateHealthLocked(); err != nil {
		return err
	}
	return d.state.ensureKey(d.saveStateLocked)
}
