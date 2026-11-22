//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"fintex.au/mnet/internal/daemon"
	"golang.org/x/sys/unix"
	"io"
	"time"
)

// One parent lane services the two framed directions and the retained private
// WG owner. Public pair metadata cannot mint the bridge; only daemon's real
// factory, dev/tun pointers and exclusive config readback issue that owner.
// Genuine child completion remains blocked by the unfinished policy rows.
func (p *endpointParentSource) serveSource(ctx context.Context) (ret error) {
	if p == nil || p.admission == nil || p.bridge == nil || p.policy == nil {
		return afPhase
	}
	defer func() { ret = errors.Join(ret, p.stopSource(ctx)) }()
	for {
		if e := p.currentSource(ctx); e != nil {
			return e
		}
		f, input, e := p.wire.readSource()
		if e != nil {
			return e
		}
		if input {
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
				v, e := p.policy.admit(f.payload, packetToPeer)
				if e != nil {
					endpointCount(&p.counts.DatagramsDropped)
				} else {
					if e = p.currentSource(ctx); e != nil {
						return e
					}
					e = p.bridge.SendToPeer(p.peer, v.bytes)
					if errors.Is(e, daemon.ErrDevelopmentEndpointBackpressure) {
						endpointCount(&p.counts.DatagramsDropped)
					} else if e != nil {
						return e
					} else {
						endpointCount(&p.counts.DatagramsToPeer)
					}
				}
			default:
				endpointCount(&p.counts.FramingErrors)
				return errEndpointFrame
			}
		}
		produced := false
		// One still-unwritten atomic pipe frame plus bounded private WG queues. Do
		// not block flushing data: STOPPED/input must still be read under pressure.
		if p.wire.pending == nil {
			packet, ok, e := p.bridge.TakeFromPeer(p.peer)
			if e != nil {
				return e
			}
			produced = ok
			if ok {
				v, e := p.policy.admit(packet, packetToKernel)
				if e != nil {
					endpointCount(&p.counts.DatagramsDropped)
				} else {
					if e = p.currentSource(ctx); e != nil {
						return e
					}
					if e = p.wire.queueSource(endpointDatagram, v.bytes); e != nil {
						return e
					}
				}
			}
		}
		wrote := false
		if p.wire.pending != nil {
			if e = p.currentSource(ctx); e != nil {
				return e
			}
			wrote, e = p.wire.flushSource()
			if e != nil {
				return e
			}
			if wrote {
				endpointCount(&p.counts.DatagramsToChild)
			}
		}
		if !input && !produced && !wrote && len(p.wire.ready) == 0 {
			if e = p.waitDataplaneSource(ctx); e != nil {
				return e
			}
		}
	}
}

// Only RawConn file references cross ppoll. No controller, daemon, native,
// queue or original-TUN mutex is held. The borrowed wake descriptor is eventfd,
// never an original TUN or WG socket. Cancellation signals it before closing
// a borrowed output descriptor; an original-capped 50ms wait remains the
// fallback for a failed/coalesced wake, not the normal packet latency.
func (p *endpointParentSource) waitDataplaneSource(ctx context.Context) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if p == nil || p.wire == nil || p.bridge == nil || ctx == nil {
		return ErrConflict
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := p.lifetime.Err(); e != nil {
		return e
	}
	if e := p.bridge.Context().Err(); e != nil {
		return e
	}
	d := endpointIdleSafetyBudget
	for _, c := range []context.Context{ctx, p.lifetime, p.bridge.Context()} {
		if end, ok := c.Deadline(); ok && time.Until(end) < d {
			d = time.Until(end)
		}
	}
	if d <= 0 {
		return context.DeadlineExceeded
	}
	wake := p.bridge.WakeFile()
	if wake == nil {
		return ErrConflict
	}
	raw, e := wake.SyscallConn()
	if e != nil {
		return e
	}
	var call error
	e = raw.Control(func(notify uintptr) {
		if p.wire.pending != nil {
			// A queued WG packet must not cause a readable-eventfd busy loop while
			// the child pipe is full. Clear only the counter, not its packet queue.
			flags, x := unix.FcntlInt(notify, unix.F_GETFL, 0)
			if x != nil || flags&unix.O_NONBLOCK == 0 {
				call = ErrConflict
				return
			}
			var counter [8]byte
			n, x := unix.Read(int(notify), counter[:])
			if x != nil && x != unix.EAGAIN && x != unix.EINTR {
				call = x
				return
			}
			if x == nil && n != 8 {
				call = ErrConflict
				return
			}
			if x = p.lifetime.Err(); x != nil {
				call = x
				return
			}
			if x = p.bridge.Context().Err(); x != nil {
				call = x
				return
			}
			if x = ctx.Err(); x != nil {
				call = x
				return
			}
		}
		call = p.wire.controlSource(p.wire.in, p.wire.readID, func(input int) error {
			return p.wire.controlSource(p.wire.out, p.wire.writeID, func(output int) error {
				fds := []unix.PollFd{{Fd: int32(input), Events: unix.POLLIN}, {Fd: int32(notify), Events: unix.POLLIN}, {Fd: int32(output)}}
				if p.wire.pending != nil {
					fds[2].Events = unix.POLLOUT
				}
				d = endpointIdleSafetyBudget
				for _, c := range []context.Context{ctx, p.lifetime, p.bridge.Context()} {
					if e := c.Err(); e != nil {
						return e
					}
					if end, ok := c.Deadline(); ok && time.Until(end) < d {
						d = time.Until(end)
					}
				}
				if d <= 0 {
					return context.DeadlineExceeded
				}
				timeout := unix.NsecToTimespec(d.Nanoseconds())
				_, e := unix.Ppoll(fds, &timeout, nil)
				if e == unix.EINTR {
					return nil
				}
				if e != nil {
					return e
				}
				if fds[0].Revents&unix.POLLIN != 0 {
					return nil
				}
				if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
					return io.EOF
				}
				if fds[1].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 || fds[2].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
					return ErrConflict
				}
				return nil
			})
		})
	})
	if e = errors.Join(e, call); e != nil {
		return e
	}
	if e = p.lifetime.Err(); e != nil {
		return e
	}
	if e = p.bridge.Context().Err(); e != nil {
		return e
	}
	return ctx.Err()
}
