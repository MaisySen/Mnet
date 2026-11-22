//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"net/netip"
)

var errEndpointNoProgress = errors.New("endpoint-nonblocking-no-progress")

// Source-only resident-thread reactor. No goroutine, pipe, launcher or IPC call.
// Constructed dormant from actual preparation. Only consumed genuine completion
// can arm I/O. Current incomplete policy still prevents any such receipt.
type originalEndpointReactor struct {
	owner    *operation1Controller
	evidence *sourceRuntimeEvidence
	receipt  *completionReceipt
	frames   *endpointFrameState
}

func newOriginalEndpointReactor(c *operation1Controller, p *preparedEndpointObservation) (*originalEndpointReactor, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if c == nil || p == nil || p.local == nil || !p.preparation.current(c.tx) || p.local.resources == nil || p.local.resources.owner != c.tx || p.local.routed == nil || !p.local.routed.current() || p.local.observedMTU != MTU {
		return nil, afPhase
	}
	e := p.local
	own, err := netip.ParseAddr(c.tx.plan.Binding.OwnIP)
	if err != nil {
		return nil, err
	}
	peer, err := netip.ParseAddr(c.tx.plan.Binding.PeerIP)
	if err != nil {
		return nil, err
	}
	policy, err := newEndpointPacketPolicy(packetFramingReadback{Own: own, Peer: peer, LocalMTU: int(e.observedMTU), PeerMTU: MTU, TUN: true, NoPI: true, NoVNET: true, NoOffload: true})
	if err != nil {
		return nil, err
	}
	frames, err := newEndpointFrameState(e.routed.identity.generation, 1, policy)
	if err != nil {
		return nil, err
	}
	return &originalEndpointReactor{owner: c, evidence: e, frames: frames}, nil
}

// Arm only after genuine mandatory completion. A dormant gate is not Ready.
func (r *originalEndpointReactor) armSource(ctx context.Context, e *sourceRuntimeEvidence, receipt *completionReceipt) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if r == nil || r.owner == nil {
		return afPhase
	}
	return r.owner.withOwner(ctx, func(ctx context.Context) error {
		c := r.owner
		if e == nil || receipt == nil || r.receipt != nil || !receipt.used || receipt.evidence != e || receipt.owner != c.tx || receipt.authority != c.tx.completionAuthority || !c.tx.Ready || c.tx.stopStarted || !receipt.ledger.matches(receipt.identity) || e.resources != r.evidence.resources || receipt.identity.generation != r.frames.generation || e.observedMTU != MTU {
			return afPhase
		}
		if err := requireEndpointPeerBinding(e.peer, c.tx.plan.Binding); err != nil {
			return err
		}
		if err := c.tx.live(ctx); err != nil {
			return err
		}
		r.evidence = e
		r.receipt = receipt
		return nil
	})
}

// The only actual dataplane entry. Controller lane then original file mutex.
// Control callback keeps the number private. Exactly one nonblocking read/write,
// no RawConn.Read wait, no Fd/dup/export/reopen or namespace switch.
func (r *originalEndpointReactor) originalIOSource(ctx context.Context, in []byte, write bool) (out []byte, err error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if r == nil || r.owner == nil || r.evidence == nil || r.frames == nil || r.receipt == nil {
		return nil, afPhase
	}
	err = r.owner.withOwner(ctx, func(ctx context.Context) error {
		c := r.owner
		e := r.evidence
		o := e.resources
		if !c.tx.Ready || c.tx.stopStarted || c.tx.fdCloseAttempted || r.receipt.owner != c.tx || r.receipt.evidence != e || !r.receipt.used || !r.receipt.ledger.matches(r.receipt.identity) {
			return afPhase
		}
		if err := c.tx.live(ctx); err != nil {
			return err
		}
		if err := requireEndpointPeerBinding(e.peer, c.tx.plan.Binding); err != nil {
			return err
		}
		o.creation.fd.mu.Lock()
		defer o.creation.fd.mu.Unlock()
		if o.creation.fd.closed || o.creation.fd.file == nil || !e.offload.matches(o.creation.attempt, o.creation.fd, o.namespace.identity, c.tx.plan.Binding.Name) {
			return afPhase
		}
		if err := o.namespace.checkCurrent(); err != nil {
			return err
		}
		raw, err := o.creation.fd.file.SyscallConn()
		if err != nil {
			return err
		}
		buffer := make([]byte, 65536)
		if write {
			v, err := r.frames.policy.admit(in, packetToKernel)
			if err != nil {
				return err
			}
			buffer = v.bytes
		}
		var callErr error
		var n int
		err = raw.Control(func(fd uintptr) {
			flags, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
			if err != nil || flags&unix.O_NONBLOCK == 0 {
				callErr = ErrConflict
				return
			}
			before, err := o.inspectOriginalControlSource(ctx, int(fd))
			if err != nil || before.identity != r.receipt.identity {
				callErr = ErrConflict
				return
			}
			if write {
				n, callErr = unix.Write(int(fd), buffer)
			} else {
				n, callErr = unix.Read(int(fd), buffer)
			}
			after, err := o.inspectOriginalControlSource(ctx, int(fd))
			if err != nil || after.identity != before.identity {
				callErr = errors.Join(callErr, ErrConflict)
			}
			final, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
			if err != nil || final != flags {
				callErr = errors.Join(callErr, ErrConflict)
			}
		})
		if err != nil {
			callErr = errors.Join(callErr, err)
		}
		if callErr == unix.EAGAIN && n <= 0 {
			return errEndpointNoProgress
		}
		if callErr != nil || (write && n != len(buffer)) || (!write && (n <= 0 || n > len(buffer))) {
			c.tx.Ready = false
			r.frames.stop()
			_ = o.ledger.invalidate("custody-loss")
			return errors.Join(callErr, ErrConflict)
		}
		if err := c.tx.live(ctx); err != nil {
			r.frames.stop()
			c.tx.Ready = false
			return err
		}
		if !write {
			out = append([]byte(nil), buffer[:n]...)
		}
		return nil
	})
	return out, err
}
func (r *originalEndpointReactor) drainOneSource(ctx context.Context) error {
	b, err := r.originalIOSource(ctx, nil, false)
	if err != nil {
		return err
	}
	return r.frames.enqueue(endpointDatagram, b) // full datagram then actual1280 byte gate
}
func (r *originalEndpointReactor) receiveSource(ctx context.Context, f endpointFrame) error {
	if r == nil || r.frames == nil {
		return afPhase
	}
	p, err := r.frames.receive(f)
	if err != nil {
		return err
	}
	if f.kind != endpointDatagram {
		return nil
	}
	_, err = r.originalIOSource(ctx, p.bytes, true)
	return err
}

// Stop orchestration must run on the same still-held resident TID. It uses the
// caller's actual cleanup context, never silently replaces cancelled authority.
// Original close/absence remains owned by stopOwnedLocked and its sticky errors.
func (r *originalEndpointReactor) stopSource(ctx context.Context) error {
	if r == nil || r.owner == nil {
		return afPhase
	}
	r.frames.stop()
	return r.owner.stop(ctx)
}
