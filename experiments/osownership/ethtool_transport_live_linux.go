//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/binary"
	"golang.org/x/sys/unix"
	"sync"
	"time"
)

// Owned read-only NETLINK_GENERIC socket. Same socket/port and increasing seq for
// family discovery, feature names and active bitmap. No mutation or multicast.
type ownedEthtoolSocket struct {
	mu            sync.Mutex
	ns            *namespaceCustody
	fd            int
	pid, sequence uint32
	family        uint16
	closed        bool
}

func openOwnedEthtoolSource(ns *namespaceCustody) (*ownedEthtoolSocket, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ns == nil {
		return nil, ErrConflict
	}
	if e := ns.checkCurrent(); e != nil {
		return nil, e
	}
	fd, e := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.NETLINK_GENERIC)
	if e != nil {
		return nil, e
	}
	keep := false
	defer func() {
		if !keep {
			unix.Close(fd)
		}
	}()
	if e = unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); e != nil {
		return nil, e
	}
	address, e := unix.Getsockname(fd)
	if e != nil {
		return nil, e
	}
	bound, ok := address.(*unix.SockaddrNetlink)
	if !ok || bound.Pid == 0 {
		return nil, ErrConflict
	}
	keep = true
	return &ownedEthtoolSocket{ns: ns, fd: fd, pid: bound.Pid}, nil
}
func (s *ownedEthtoolSocket) closeSource() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return unix.Close(s.fd)
}
func (s *ownedEthtoolSocket) next() (uint32, error) {
	if s == nil || s.closed || s.sequence == ^uint32(0) {
		return 0, ErrConflict
	}
	s.sequence++
	return s.sequence, nil
}

// Called only under socket lock by exact builders below. Invalid replies poison
// the whole socket, no retries that could misassociate a prior sequence.
func (s *ownedEthtoolSocket) exchangeLocked(ctx context.Context, request []byte) (Datagram, error) {
	var zero Datagram
	if !kernelExecutionReleased {
		return zero, ErrExecutorUnreleased
	}
	if s == nil || s.closed || ctx == nil || ctx.Err() != nil || len(request) < 20 || len(request) > 4096 {
		return zero, ErrConflict
	}
	family := binary.NativeEndian.Uint16(request[4:])
	cmd := request[16]
	if binary.NativeEndian.Uint32(request) != uint32(len(request)) || binary.NativeEndian.Uint16(request[6:]) != 1 || binary.NativeEndian.Uint32(request[8:]) != s.sequence || binary.NativeEndian.Uint32(request[12:]) != s.pid || request[18] != 0 || request[19] != 0 {
		return zero, afRequest
	}
	if !((family == 16 && cmd == 3 && request[17] == 2) || (s.family >= 16 && family == s.family && (cmd == 1 || cmd == 11) && request[17] == 1)) {
		return zero, afRequest
	}
	if e := s.ns.checkCurrent(); e != nil {
		return zero, e
	}
	bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if e := unix.Sendto(s.fd, request, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); e != nil {
		return zero, e
	}
	for {
		if e := bounded.Err(); e != nil {
			return zero, e
		}
		if e := s.ns.checkCurrent(); e != nil {
			return zero, e
		}
		n, e := unix.Poll([]unix.PollFd{{Fd: int32(s.fd), Events: unix.POLLIN}}, 10)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return zero, e
		}
		if n == 0 {
			continue
		}
		buffer := make([]byte, 4096)
		size, _, flags, from, e := unix.Recvmsg(s.fd, buffer, nil, 0)
		if e == unix.EAGAIN || e == unix.EINTR {
			continue
		}
		if e != nil {
			return zero, e
		}
		sender, ok := from.(*unix.SockaddrNetlink)
		if !ok || sender.Pid != 0 || flags&unix.MSG_TRUNC != 0 || size < 20 {
			return zero, afRequest
		}
		return Datagram{Bytes: buffer[:size], SenderPID: sender.Pid, Truncated: false}, nil
	}
}
func (s *ownedEthtoolSocket) collectFeaturesSource(ctx context.Context, index uint32, name string) (names *verifiedFeatureNames, features ethtoolActiveFeatures, err error) {
	if !kernelExecutionReleased {
		return nil, features, ErrExecutorUnreleased
	}
	if s == nil {
		return nil, features, ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if err != nil && !s.closed {
			s.closed = true
			_ = unix.Close(s.fd)
		}
	}()
	if s.family != 0 {
		return nil, features, ErrConflict
	} // one collection per owned socket
	seq, e := s.next()
	if e != nil {
		return nil, features, e
	}
	request, e := buildEthtoolFamilyRequest(seq, s.pid)
	if e != nil {
		return nil, features, e
	}
	reply, e := s.exchangeLocked(ctx, request)
	if e != nil {
		return nil, features, e
	}
	s.family, e = parseEthtoolFamilyReply(reply, seq, s.pid)
	if e != nil {
		return nil, features, e
	}
	seq, e = s.next()
	if e != nil {
		return nil, features, e
	}
	expected := ethtoolFeatureRequest{family: s.family, sequence: seq, recipient: s.pid, ifindex: index, name: name, profile: pinnedFeaturesV617}
	request, e = buildFeatureNamesRequest(expected)
	if e != nil {
		return nil, features, e
	}
	reply, e = s.exchangeLocked(ctx, request)
	if e != nil {
		return nil, features, e
	}
	names, e = parseFeatureNames(reply, expected)
	if e != nil {
		return nil, features, e
	}
	seq, e = s.next()
	if e != nil {
		return nil, features, e
	}
	expected.sequence = seq
	request, e = buildEthtoolFeatureRequest(expected)
	if e != nil {
		return nil, features, e
	}
	reply, e = s.exchangeLocked(ctx, request)
	if e != nil {
		return nil, features, e
	}
	features, e = parseEthtoolActive(reply, expected)
	if e != nil {
		return nil, features, e
	}
	return names, features, nil
}
