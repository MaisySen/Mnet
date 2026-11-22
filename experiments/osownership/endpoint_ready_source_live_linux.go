//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"time"
)

func (s *operation1ResidentSource) ownerThreadCurrent() bool {
	return s != nil && s.ns != nil && unix.Gettid() == s.ns.threadID
}

// Actual fixed bootstrap owns this controller on exactly one locked TID. Every
// effect entry rejects other TIDs BEFORE its mutex; timers only cancel context
// or close the parent pipe, and no child bridge worker gets this controller.
// Thus no competing effect waiter can prevent this bounded wait from returning
// to stop. Parent pipe data/HUP wakes immediately, original expiry caps timeout.
// The original mutex+RawConn.Control keep original FD private and nonclosing.
// This is an explicit wait contract, NOT removal of any per-I/O custody check.
func (r *originalEndpointReactor) waitReadySource(ctx context.Context, w *endpointPipeWire, wait bool) (ready endpointPollResult, ret error) {
	if !kernelExecutionReleased {
		return ready, ErrExecutorUnreleased
	}
	if r == nil || r.owner == nil || r.owner.lane == nil || !r.owner.lane.ownerThreadCurrent() || r.receipt == nil || r.frames == nil || r.evidence == nil || r.evidence.resources == nil || w == nil {
		return ready, afPhase
	}
	ret = r.owner.withOwner(ctx, func(lease context.Context) error {
		c := r.owner
		o := r.evidence.resources
		if c.tx == nil || o.creation == nil || o.creation.fd == nil || o.namespace == nil {
			return afPhase
		}
		if !c.tx.Ready || c.tx.stopStarted || !r.receipt.ledger.matches(r.receipt.identity) || r.admission == nil {
			return afPhase
		}
		if e := c.tx.live(lease); e != nil {
			return e
		}
		if e := r.admission.checkResidentSource(lease, o.namespace, c.tx.plan.Binding, r.receipt.identity.generation); e != nil {
			return e
		}
		pollDelay := func() (time.Duration, error) {
			d := time.Duration(0)
			if wait {
				d = endpointIdleSafetyBudget
			}
			if end, ok := lease.Deadline(); ok {
				left := time.Until(end)
				if left <= 0 {
					return 0, context.DeadlineExceeded
				}
				if wait && left < d {
					d = left
				}
			}
			return d, nil
		}
		d, e := pollDelay()
		if e != nil {
			return e
		}
		room := r.frames.datagramRoom()
		o.creation.fd.mu.Lock()
		defer o.creation.fd.mu.Unlock()
		if o.creation.fd.closed || o.creation.fd.file == nil {
			return afPhase
		}
		raw, e := o.creation.fd.file.SyscallConn()
		if e != nil {
			return e
		}
		var call error
		e = raw.Control(func(fd uintptr) {
			flags, x := unix.FcntlInt(fd, unix.F_GETFL, 0)
			if x != nil || flags&unix.O_NONBLOCK == 0 {
				call = ErrConflict
				return
			}
			before, x := o.inspectOriginalControlSource(lease, int(fd))
			if x != nil || before.identity != r.receipt.identity {
				call = ErrConflict
				return
			}
			call = w.controlSource(w.in, w.readID, func(input int) error {
				return w.controlSource(w.out, w.writeID, func(output int) error {
					events := int16(0)
					if room {
						events = unix.POLLIN
					}
					fds := []unix.PollFd{{Fd: int32(fd), Events: events}, {Fd: int32(input), Events: unix.POLLIN}, {Fd: int32(output)}}
					if w.pending != nil {
						fds[2].Events = unix.POLLOUT
					}
					if x := lease.Err(); x != nil {
						return x
					}
					d, x = pollDelay()
					if x != nil {
						return x
					}
					timeout := unix.NsecToTimespec(d.Nanoseconds())
					_, err := unix.Ppoll(fds, &timeout, nil)
					if err == unix.EINTR {
						return nil
					}
					if err != nil {
						return err
					}
					if fds[1].Revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
						return ErrConflict
					}
					if fds[1].Revents&unix.POLLHUP != 0 {
						return errEndpointInputClosed
					}
					if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 || fds[2].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
						return ErrConflict
					}
					ready = endpointPollResult{TUN: fds[0].Revents, Input: fds[1].Revents, Output: fds[2].Revents}
					return nil
				})
			})
			after, x := o.inspectOriginalControlSource(lease, int(fd))
			if x != nil || after.identity != before.identity {
				call = errors.Join(call, ErrConflict)
			}
			final, x := unix.FcntlInt(fd, unix.F_GETFL, 0)
			if x != nil || final != flags {
				call = errors.Join(call, ErrConflict)
			}
		})
		if e = errors.Join(e, call); e != nil {
			return e
		}
		return c.tx.live(lease)
	})
	if ret != nil {
		return endpointPollResult{}, ret
	}
	return ready, nil
}
