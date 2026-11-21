//go:build linux && mnet_enrollment_development

package osownership

// Live transport source is mechanically unreleased. No tests construct it.
import (
	"context"
	"encoding/binary"
	"errors"
	"golang.org/x/sys/unix"
	"sync"
	"time"
)

type routeSocket struct {
	mu     sync.Mutex
	ns     *namespaceCustody
	fd     int
	pid    uint32
	closed bool
}

func (n *namespaceCustody) checkCurrent() error {
	if n == nil || n.fd < 0 {
		return ErrConflict
	}
	var target unix.Stat_t
	if e := unix.Fstat(n.fd, &target); e != nil || uint64(target.Dev) != n.identity.Device || target.Ino != n.identity.Inode {
		return ErrConflict
	}
	fd, e := unix.Open("/proc/thread-self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	var here unix.Stat_t
	if e = unix.Fstat(fd, &here); e != nil || here.Dev != target.Dev || here.Ino != target.Ino {
		return ErrConflict
	}
	return nil
}
func openRouteSocket(ns *namespaceCustody) (*routeSocket, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if e := ns.checkCurrent(); e != nil {
		return nil, e
	}
	fd, e := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_ROUTE)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			unix.Close(fd)
		}
	}()
	if e = unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); e != nil {
		return nil, e
	}
	a, e := unix.Getsockname(fd)
	if e != nil {
		return nil, e
	}
	nl, yes := a.(*unix.SockaddrNetlink)
	if !yes || nl.Pid == 0 {
		return nil, errors.New("invalid netlink port ID")
	}
	ok = true
	return &routeSocket{ns: ns, fd: fd, pid: nl.Pid}, nil
}
func (s *routeSocket) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return unix.Close(s.fd)
}
func (s *routeSocket) Exchange(ctx context.Context, request []byte) (out []Datagram, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if err != nil && !s.closed {
			s.closed = true
			_ = unix.Close(s.fd)
		}
	}()
	if s.closed {
		return nil, errors.New("netlink closed")
	}
	if e := s.ns.checkCurrent(); e != nil {
		return nil, e
	}
	if len(request) < 16 || len(request) > 65536 {
		return nil, errors.New("invalid netlink request")
	}
	bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if e := unix.Sendto(s.fd, request, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); e != nil {
		return nil, e
	}
	var result []Datagram
	total := 0
	for {
		if e := bounded.Err(); e != nil {
			return nil, e
		}
		if e := s.ns.checkCurrent(); e != nil {
			return nil, e
		}
		polled, e := unix.Poll([]unix.PollFd{{Fd: int32(s.fd), Events: unix.POLLIN}}, 10)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return nil, e
		}
		if polled == 0 {
			continue
		}
		buf := make([]byte, 65536)
		n, _, flags, from, e := unix.Recvmsg(s.fd, buf, nil, 0)
		if e == unix.EAGAIN || e == unix.EINTR {
			continue
		}
		if e != nil {
			return nil, e
		}
		nl, ok := from.(*unix.SockaddrNetlink)
		if !ok {
			return nil, errors.New("non-netlink sender")
		}
		total += n
		if total > maxNetlinkBytes || n == 0 {
			return nil, errors.New("empty or excessive netlink dump")
		}
		d := Datagram{Bytes: buf[:n], SenderPID: nl.Pid, Truncated: flags&unix.MSG_TRUNC != 0}
		result = append(result, d)
		// Only termination detection here; full framing/origin/seq validation in
		// client must succeed before any body is accepted. Never ignore trailing data.
		for b := d.Bytes; len(b) > 0; {
			if len(b) < 16 {
				return nil, errors.New("short trailing live frame")
			}
			size := int(binary.NativeEndian.Uint32(b))
			if size < 16 || size > len(b) {
				return nil, errors.New("invalid live frame")
			}
			typ := binary.NativeEndian.Uint16(b[4:])
			if typ == nlError || typ == nlDone {
				return result, nil
			}
			size = (size + 3) &^ 3
			if size > len(b) {
				return nil, errors.New("invalid live alignment")
			}
			b = b[size:]
		}
	}
}
