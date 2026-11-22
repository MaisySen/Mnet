//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
)

// Required local disable collection, NOT the optional full CONFIG diagnostic
// collector. Every namespace/device check remains held-original and resident.
func (o *originalHeldPacketResources) collectDisableSource(ctx context.Context) (RawInventory, SysctlSnapshot, error) {
	if !kernelExecutionReleased {
		return RawInventory{}, nil, ErrExecutorUnreleased
	}
	if o == nil || o.namespace == nil || o.session == nil || o.session.canonical == nil {
		return RawInventory{}, nil, ErrConflict
	}
	raw, e := ReadStableInventory(ctx, ownedSetupDump{residentNetlink: residentNetlink{ns: o.namespace}, handle: *o.session.canonical}, o.namespace.checkCurrent)
	if e != nil {
		return RawInventory{}, nil, e
	}
	f, e := o.openDisableLeafSource(false)
	if e != nil {
		return RawInventory{}, nil, e
	}
	b, re := io.ReadAll(io.LimitReader(f, 17))
	ce := f.Close()
	if e = errors.Join(re, ce); e != nil {
		return RawInventory{}, nil, e
	}
	v := strings.TrimSpace(string(b))
	if len(b) > 16 || (v != "0" && v != "1") {
		return RawInventory{}, nil, afPolicy
	}
	if e = o.namespace.checkCurrent(); e != nil {
		return RawInventory{}, nil, e
	}
	return raw, SysctlSnapshot{"/proc/sys/net/ipv6/conf/" + o.session.canonical.Link.Name + "/disable_ipv6": v}, nil
}
func (o *originalHeldPacketResources) openDisableLeafSource(write bool) (result *os.File, err error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if o == nil || o.namespace == nil || o.session == nil || o.session.canonical == nil {
		return nil, ErrConflict
	}
	name := o.session.canonical.Link.Name
	if !safeInterfaceName(name) {
		return nil, ErrConflict
	}
	if e := o.namespace.checkCurrent(); e != nil {
		return nil, e
	}
	root, e := unix.Open("/proc/sys/net/ipv6/conf", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer func() {
		err = errors.Join(err, unix.Close(root))
		if err != nil && result != nil {
			err = errors.Join(err, result.Close())
			result = nil
		}
	}()
	dir, e := unix.Openat(root, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer func() { err = errors.Join(err, unix.Close(dir)) }()
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if write {
		flags = unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW
	}
	fd, e := unix.Openat(dir, "disable_ipv6", flags, 0)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), "owned-disable-required-leaf"), nil
}

// Source-only handoff. Controller owns lane; file mutex spans identity, capture,
// one write attempt and independent AF+address+route readback. Called only through
// the private adapter source interface; no generic Inventory conversion.
func (s *operation1ResidentSource) disableFreshDownSource(ctx context.Context, o *originalHeldPacketResources) (result *postDisableResult, err error) {
	piece, finish, pieceErr := s.beginForwardPieceSource(ctx)
	if pieceErr != nil {
		return result, pieceErr
	}
	ctx = piece
	defer func() {
		err = finish(err)
		if err != nil {
			result = nil
		}
	}()

	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if s == nil || o == nil || s.owner == nil || o.owner != s.owner.tx || o.session != s.owner.adapter.session || o.namespace != s.ns || o.creation == nil || o.creation.fd == nil || s.disableLife == nil || !s.disableLife.current() {
		return nil, afPhase
	}
	if e := s.checkForwardSource(ctx, s.owner.attempt); e != nil {
		return nil, e
	}
	if s.owner.adapter == nil || !s.owner.adapter.authorized(ctx) || o.creation.fd != s.owner.attempt.custody {
		return nil, afPhase
	}
	// disableAttempted is protected by the authorized controller lane above,
	// not by lifetime atomics or the later file mutex.
	if s.disableAttempted {
		return nil, afPhase
	}
	s.disableAttempted = true
	o.creation.fd.mu.Lock()
	defer o.creation.fd.mu.Unlock()
	before, e := o.verifyOriginalDeviceLockedSource(ctx)
	if e != nil {
		return nil, e
	}
	held, e := heldSourceSnapshot(o.ledger, o.owner, o.session)
	if e != nil {
		return nil, e
	}
	raw, sys, e := o.collectDisableSource(ctx)
	if e != nil {
		return nil, e
	}
	d := ownedSourceDiagnostic{raw: raw, sys: sys, held: held, lineageUsable: o.ledger.matches(held.identity)}
	check := func() error {
		if e := s.checkForwardSource(ctx, s.owner.attempt); e != nil {
			return e
		}
		after, e := o.verifyOriginalDeviceLockedSource(ctx)
		if e != nil {
			return e
		}
		if !sameDisableOriginalIdentity(after.identity, before.identity) || after.deviceNamespace != before.deviceNamespace || after.tunName != before.tunName {
			return ErrConflict
		}
		return nil
	}
	return finishOwnedDisable(ctx, s.disableLife, o.ledger, d, ownedDisableDependencies{check: check, leaf: func() (ownedDisableLeaf, error) { return o.openDisableLeafSource(true) }, collect: o.collectDisableSource})
}
