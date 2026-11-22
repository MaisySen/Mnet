//go:build linux && mnet_enrollment_development

package daemon

import (
	"encoding/binary"
	"golang.org/x/sys/unix"
	"os"
	"sync"
	"sync/atomic"
)

type developmentEndpointEvent struct {
	file   *os.File
	once   sync.Once
	closed atomic.Bool
	err    error
}

func newDevelopmentEndpointNotifier() (developmentEndpointNotifier, error) {
	fd, e := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "private-endpoint-wake")
	if f == nil {
		unix.Close(fd)
		return nil, errPrivateEndpoint
	}
	return &developmentEndpointEvent{file: f}, nil
}
func (n *developmentEndpointEvent) File() *os.File { return n.file }
func (n *developmentEndpointEvent) call(fn func(int) error) error {
	if n == nil || n.closed.Load() {
		return os.ErrClosed
	}
	raw, e := n.file.SyscallConn()
	if e != nil {
		return e
	}
	var call error
	e = raw.Control(func(fd uintptr) {
		flags, x := unix.FcntlInt(fd, unix.F_GETFL, 0)
		clo, y := unix.FcntlInt(fd, unix.F_GETFD, 0)
		if x != nil || y != nil || flags&unix.O_NONBLOCK == 0 || clo&unix.FD_CLOEXEC == 0 {
			call = errPrivateEndpoint
			return
		}
		call = fn(int(fd))
	})
	if e != nil {
		return e
	}
	return call
}
func (n *developmentEndpointEvent) Signal() error {
	var buf [8]byte
	binary.NativeEndian.PutUint64(buf[:], 1)
	e := n.call(func(fd int) error {
		count, e := unix.Write(fd, buf[:])
		if e == unix.EAGAIN || e == unix.EINTR {
			return nil
		}
		if e != nil {
			return e
		}
		if count != 8 {
			return errPrivateEndpoint
		}
		return nil
	})
	return e
}
func (n *developmentEndpointEvent) Drain() error {
	var buf [8]byte
	return n.call(func(fd int) error {
		count, e := unix.Read(fd, buf[:])
		if e == unix.EAGAIN || e == unix.EINTR {
			return nil
		}
		if e != nil {
			return e
		}
		if count != 8 {
			return errPrivateEndpoint
		}
		return nil
	})
}
func (n *developmentEndpointEvent) Close() error {
	if n == nil {
		return nil
	}
	n.once.Do(func() { _ = n.Signal(); n.closed.Store(true); n.err = n.file.Close() })
	return n.err
}
