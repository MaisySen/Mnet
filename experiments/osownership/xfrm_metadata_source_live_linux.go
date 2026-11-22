//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"time"
)

// Only this private source producer binds metadata DATA to an original object,
// ledger generation and same held namespace/TID/Control interval. It is not a
// semantic policy witness. No tests construct it or execute this collector.
type originalXfrmMetadataEvidence struct {
	resources *originalHeldPacketResources
	identity  heldPacketIdentity
	data      xfrmMetadataData
}

func (e *originalXfrmMetadataEvidence) matches(o *originalHeldPacketResources, id heldPacketIdentity) bool {
	return e != nil && o != nil && o.ledger != nil && e.resources == o && e.identity == id && o.ledger.matches(id) && e.data.emptyAcceptDefaults()
}

// Caller already holds the original FD mutex. Control retains its reference
// across the complete observation, with independent original identity on both
// sides. No descriptor/name-based replacement or caller snapshot is accepted.
func (o *originalHeldPacketResources) collectOriginalXfrmLockedSource(ctx context.Context, id heldPacketIdentity) (out *originalXfrmMetadataEvidence, ret error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || o == nil || o.creation == nil || o.creation.fd == nil || o.creation.fd.closed || o.creation.fd.file == nil || o.namespace == nil || o.ledger == nil || !o.ledger.matches(id) {
		return nil, afPhase
	}
	raw, e := o.creation.fd.file.SyscallConn()
	if e != nil {
		return nil, e
	}
	var call error
	ret = raw.Control(func(fd uintptr) { out, call = o.collectOriginalXfrmControlSource(ctx, int(fd), id) })
	ret = errors.Join(ret, call)
	if ret != nil {
		return nil, ret
	}
	if out == nil {
		return nil, afPhase
	}
	return out, nil
}

// Actual I/O calls this inside its already retained original Control interval;
// no nested Control, FD handoff or mutex reacquisition is needed.
func (o *originalHeldPacketResources) collectOriginalXfrmControlSource(ctx context.Context, fd int, id heldPacketIdentity) (*originalXfrmMetadataEvidence, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || o == nil || o.creation == nil || o.creation.fd == nil || o.creation.fd.closed || o.creation.fd.file == nil || o.namespace == nil || o.ledger == nil || !o.ledger.matches(id) {
		return nil, afPhase
	}
	if e := o.namespace.checkCurrent(); e != nil {
		return nil, e
	}
	before, e := o.inspectOriginalControlSource(ctx, fd)
	if e != nil || before.identity != id {
		return nil, ErrConflict
	}
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if e != nil || flags&unix.O_NONBLOCK == 0 {
		return nil, ErrConflict
	}
	data, call := collectXfrmMetadataSource(ctx, o.namespace)
	after, e := o.inspectOriginalControlSource(ctx, fd)
	if e != nil || after.identity != before.identity {
		call = errors.Join(call, ErrConflict)
	}
	final, e := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if e != nil || final != flags {
		call = errors.Join(call, ErrConflict)
	}
	call = errors.Join(call, ctx.Err())
	if !o.ledger.matches(id) {
		call = errors.Join(call, ErrConflict)
	}
	if call != nil {
		return nil, call
	}
	return &originalXfrmMetadataEvidence{resources: o, identity: id, data: data}, nil
}

// Fresh unicast-only NETLINK_XFRM socket for one six-query collection. No
// multicast membership, SA/policy body, SET, mutation, ACK or dump operation.
func collectXfrmMetadataSource(ctx context.Context, ns *namespaceCustody) (out xfrmMetadataData, ret error) {
	if !kernelExecutionReleased {
		return out, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || ns == nil {
		return out, afPhase
	}
	if e := ns.checkCurrent(); e != nil {
		return out, e
	}
	fd, e := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.NETLINK_XFRM)
	if e != nil {
		return out, e
	}
	defer func() {
		ret = errors.Join(ret, unix.Close(fd))
		if ret != nil {
			out = xfrmMetadataData{}
		}
	}()
	if e = unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); e != nil {
		return out, e
	}
	sa, e := unix.Getsockname(fd)
	if e != nil {
		return out, e
	}
	bound, ok := sa.(*unix.SockaddrNetlink)
	if !ok || bound.Family != unix.AF_NETLINK || bound.Pid == 0 || bound.Groups != 0 {
		return out, ErrConflict
	}
	var seq uint32
	exchange := func(kind uint16) (Datagram, error) {
		var zero Datagram
		seq++
		request, x := buildXfrmMetadataRequest(kind, seq, bound.Pid)
		if x != nil {
			return zero, x
		}
		if x = ns.checkCurrent(); x != nil {
			return zero, x
		}
		bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		if x = bounded.Err(); x != nil {
			return zero, x
		}
		if x = unix.Sendto(fd, request, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); x != nil {
			return zero, x
		}
		for {
			if x = bounded.Err(); x != nil {
				return zero, x
			}
			if x = ns.checkCurrent(); x != nil {
				return zero, x
			}
			fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, x := unix.Poll(fds, 10)
			if x == unix.EINTR {
				continue
			}
			if x != nil {
				return zero, x
			}
			if n == 0 {
				continue
			}
			if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
				return zero, ErrConflict
			}
			if fds[0].Revents&unix.POLLIN == 0 {
				return zero, ErrConflict
			}
			buf := make([]byte, 4096)
			size, _, flags, from, x := unix.Recvmsg(fd, buf, nil, 0)
			if x == unix.EAGAIN || x == unix.EINTR {
				continue
			}
			if x != nil {
				return zero, x
			}
			sender, ok := from.(*unix.SockaddrNetlink)
			if !ok || sender.Family != unix.AF_NETLINK || sender.Pid != 0 || sender.Groups != 0 || flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 || size < 16 {
				return zero, errXfrmMetadata
			}
			if x = bounded.Err(); x != nil {
				return zero, x
			}
			if x = ns.checkCurrent(); x != nil {
				return zero, x
			}
			// Kernel errno is reported only as a number, never a reply body. An error
			// cannot become an empty snapshot; successful unsolicited ACK is refused.
			if binary.NativeEndian.Uint16(buf[4:]) == nlError {
				body, x := parseXfrmMetadataFrame(Datagram{Bytes: buf[:size]}, nlError, seq, bound.Pid)
				if x == nil && len(body) >= 20 {
					return zero, fmt.Errorf("xfrm metadata GET 0x%x failed (netlink error %d)", kind, int32(binary.NativeEndian.Uint32(body)))
				}
				return zero, errXfrmMetadata
			}
			return Datagram{Bytes: buf[:size]}, nil
		}
	}
	var first xfrmMetadataData
	for round := 0; round < 2; round++ {
		var d xfrmMetadataData
		reply, x := exchange(xfrmGetSADMetadata)
		if x != nil {
			return out, x
		}
		d.SAD, x = parseXfrmSADMetadata(reply, seq, bound.Pid)
		if x != nil {
			return out, x
		}
		reply, x = exchange(xfrmGetSPDMetadata)
		if x != nil {
			return out, x
		}
		d.SPD, x = parseXfrmSPDMetadata(reply, seq, bound.Pid)
		if x != nil {
			return out, x
		}
		reply, x = exchange(xfrmGetDefaults)
		if x != nil {
			return out, x
		}
		d.Defaults, x = parseXfrmDefaults(reply, seq, bound.Pid)
		if x != nil {
			return out, x
		}
		if !d.emptyAcceptDefaults() {
			return out, errXfrmEnvironment
		}
		if round == 0 {
			first = d
		} else if first != d {
			return out, errXfrmEnvironment
		}
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	if e = ns.checkCurrent(); e != nil {
		return out, e
	}
	return first, nil
}
