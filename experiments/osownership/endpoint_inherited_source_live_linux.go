//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"strconv"
	"time"
)

type residentEndpointAdmission struct {
	wire        *endpointPipeWire
	ns          *namespaceCustody
	ctx         context.Context
	parentPID   int
	parentTicks uint64
	start       endpointStartBody
	prepared    *endpointPreparedBody
	admitted    *endpointAdmissionBody
	started     bool
}

func (*residentEndpointAdmission) endpointInheritedChannelOwner() {}
func (a *residentEndpointAdmission) checkMembershipSource(ctx context.Context, ns *namespaceCustody, b Binding) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if a == nil || a.wire == nil || a.ctx == nil || ctx == nil || a.wire.ended != nil || a.ns != ns || os.Getppid() != a.parentPID || a.start.binding() != b {
		return ErrConflict
	}
	if a.ctx.Err() != nil {
		return a.ctx.Err()
	}
	if a.wire.inputStopped {
		return errEndpointInputClosed
	}
	if e := ns.checkCurrent(); e != nil {
		return e
	}
	// No owner/file mutex is acquired for this zero-time pipe readiness check.
	// EOF/HUP revokes even when old buffered frames remain unread.
	return a.wire.controlSource(a.wire.in, a.wire.readID, func(fd int) error {
		p := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		zero := unix.Timespec{}
		_, e := unix.Ppoll(p, &zero, nil)
		if e != nil {
			return e
		}
		if p[0].Revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
			return ErrConflict
		}
		if p[0].Revents&unix.POLLHUP != 0 {
			return errEndpointInputClosed
		}
		return nil
	})
}
func (a *residentEndpointAdmission) checkResidentSource(ctx context.Context, ns *namespaceCustody, b Binding, generation uint64) error {
	if e := a.checkMembershipSource(ctx, ns, b); e != nil {
		return e
	}
	if !a.started || a.prepared == nil || a.admitted == nil || a.admitted.Prepared != *a.prepared || a.prepared.Generation != generation {
		return afPhase
	}
	return nil
}
func (a *residentEndpointAdmission) acceptSource(f endpointFrame) error {
	if a == nil || a.prepared == nil || a.admitted != nil || f.kind != endpointAdmission || f.direction != 1 || f.generation != a.prepared.Generation {
		return afPhase
	}
	b, e := decodeEndpointAdmission(f.payload)
	if e != nil || b.Prepared != *a.prepared {
		return errEndpointFrame
	}
	if e = a.checkMembershipSource(a.ctx, a.ns, a.start.binding()); e != nil {
		return e
	}
	a.admitted = &b
	return nil
}
func endpointEnvUint(k string) (uint64, error) {
	v, e := strconv.ParseUint(os.Getenv(k), 10, 64)
	if e != nil || v == 0 {
		return 0, ErrConflict
	}
	return v, nil
}
func newResidentEndpointAdmissionSource(ctx context.Context, ns *namespaceCustody, w *endpointPipeWire, start endpointStartBody) (*residentEndpointAdmission, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	pid := parseSourceParentPID()
	ticks, e := endpointEnvUint("MNO_ENDPOINT_PARENT_TICKS")
	if e != nil || pid <= 1 || os.Getppid() != pid {
		return nil, ErrConflict
	}
	actual, e := readProcessStartTicks(pid)
	if e != nil || actual != ticks {
		return nil, ErrConflict
	}
	for i, id := range []endpointPipeIdentity{w.readID, w.writeID} {
		prefix := "MNO_ENDPOINT_" + strconv.Itoa(i)
		dev, e := endpointEnvUint(prefix + "_DEV")
		if e != nil {
			return nil, e
		}
		ino, e := endpointEnvUint(prefix + "_INO")
		if e != nil || dev != id.dev || ino != id.ino {
			return nil, ErrConflict
		}
	}
	if start.Prepared.PID != uint32(os.Getpid()) || !sameNamespace(start.Prepared.Namespace, ns.identity) {
		return nil, ErrConflict
	}
	ownTicks, e := readProcessStartTicks(os.Getpid())
	if e != nil || ownTicks != start.Prepared.StartTicks {
		return nil, ErrConflict
	}
	end := time.UnixMilli(start.Prepared.SelfUntilMS)
	other := time.UnixMilli(start.Prepared.PeerUntilMS)
	if other.Before(end) {
		end = other
	}
	if !time.Now().Before(end) {
		return nil, context.DeadlineExceeded
	}
	a := &residentEndpointAdmission{wire: w, ns: ns, ctx: ctx, parentPID: pid, parentTicks: ticks, start: start}
	if e = a.checkMembershipSource(ctx, ns, start.binding()); e != nil {
		return nil, e
	}
	return a, nil
}
func withInheritedEndpointResidentSource(ctx context.Context, ns *namespaceCustody, b *CapturedBaseline, id adapterIdentity, a *residentEndpointAdmission, work func(*operation1Controller) error) (err error) {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if a == nil || b == nil || work == nil || !sameNamespace(ns.identity, b.ns) || !sameNamespace(id.Namespace, b.ns) {
		return ErrConflict
	}
	if err = a.checkMembershipSource(ctx, ns, a.start.binding()); err != nil {
		return err
	}
	c, e := newOperation1SourceController(b, id, a.start.binding())
	if e != nil {
		return e
	}
	source := &operation1ResidentSource{channel: a, disableLife: &disableResidentLifetime{}, owner: c, ns: ns, baseline: c.adapter.session.baseline, active: true}
	c.endpointCleanup = &endpointOriginalCleanupResult{owner: c}
	c.lane = source
	c.adapter.creation = source
	c.absence = source
	defer func() {
		source.disableLife.revoke()
		// Exact still-held namespace/controller/original ownership survives operation
		// cancellation. The first controller stop creates its sole cleanup interval, not
		// forward I/O or renewed membership. This defer cannot renew it.
		stopErr := source.cleanupEndpointSource(ctx)
		err = errors.Join(err, stopErr)
		c.mu.Lock()
		source.active = false
		if source.afOwner != nil {
			source.afOwner.active = false
			source.afOwner.last = nil
		}
		c.mu.Unlock()
	}()
	return work(c)
}
