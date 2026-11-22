//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type endpointPipeIdentity struct {
	dev, ino uint64
	access   int
}
type endpointPipeWire struct {
	in, out                    *os.File
	readID, writeID            endpointPipeIdentity
	inbound                    byte
	generation, received, sent uint64
	decoder                    endpointFrameDecoder
	ready                      []endpointFrame
	pending                    []byte
	ended                      error
	closed                     bool
	inputStopped               bool
	writeOnce                  sync.Once
	writeClosed                atomic.Bool
	writeErr                   error
	closeErr                   error
}

func endpointPipeIdentitySource(f *os.File, access int) (id endpointPipeIdentity, e error) {
	if f == nil {
		return id, ErrConflict
	}
	raw, e := f.SyscallConn()
	if e != nil {
		return id, e
	}
	var call error
	e = raw.Control(func(fd uintptr) {
		var st unix.Stat_t
		if call = unix.Fstat(int(fd), &st); call != nil {
			return
		}
		if st.Mode&unix.S_IFMT != unix.S_IFIFO {
			call = ErrConflict
			return
		}
		flags, x := unix.FcntlInt(fd, unix.F_GETFL, 0)
		if x != nil || flags&unix.O_ACCMODE != access {
			call = ErrConflict
			return
		}
		if call = unix.SetNonblock(int(fd), true); call != nil {
			return
		}
		if _, call = unix.FcntlInt(fd, unix.F_SETFD, unix.FD_CLOEXEC); call != nil {
			return
		}
		size, x := unix.FcntlInt(fd, unix.F_GETPIPE_SZ, 0)
		if x != nil || size < 4096 {
			call = ErrConflict
			return
		}
		id = endpointPipeIdentity{uint64(st.Dev), st.Ino, access}
	})
	return id, errors.Join(e, call)
}
func newEndpointPipeWireSource(in, out *os.File, inbound byte) (*endpointPipeWire, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if inbound != 1 && inbound != 2 {
		return nil, ErrConflict
	}
	a, e := endpointPipeIdentitySource(in, unix.O_RDONLY)
	if e != nil {
		return nil, e
	}
	b, e := endpointPipeIdentitySource(out, unix.O_WRONLY)
	if e != nil {
		return nil, e
	}
	if a.dev == b.dev && a.ino == b.ino {
		return nil, ErrConflict
	}
	return &endpointPipeWire{in: in, out: out, readID: a, writeID: b, inbound: inbound, generation: 1}, nil
}
func (w *endpointPipeWire) controlSource(f *os.File, id endpointPipeIdentity, fn func(int) error) error {
	if w == nil || w.ended != nil {
		return errEndpointStopped
	}
	raw, e := f.SyscallConn()
	if e != nil {
		return e
	}
	var call error
	e = raw.Control(func(fd uintptr) {
		var st unix.Stat_t
		if call = unix.Fstat(int(fd), &st); call != nil {
			return
		}
		flags, x := unix.FcntlInt(fd, unix.F_GETFL, 0)
		clo, y := unix.FcntlInt(fd, unix.F_GETFD, 0)
		if x != nil || y != nil || flags&unix.O_NONBLOCK == 0 || flags&unix.O_ACCMODE != id.access || clo&unix.FD_CLOEXEC == 0 || uint64(st.Dev) != id.dev || st.Ino != id.ino || st.Mode&unix.S_IFMT != unix.S_IFIFO {
			call = ErrConflict
			return
		}
		call = fn(int(fd))
	})
	return errors.Join(e, call)
}
func (w *endpointPipeWire) fail(e error) error {
	if e == nil {
		e = errEndpointFrame
	}
	if w.ended == nil {
		w.ended = e
	}
	w.ready = nil
	w.pending = nil
	if !w.closed {
		w.closed = true
		w.closeErr = errors.Join(w.closeWriteSource(), w.in.Close())
	}
	return errors.Join(e, w.closeErr)
}
func (w *endpointPipeWire) readSource() (endpointFrame, bool, error) {
	if w == nil || w.ended != nil || w.inputStopped {
		return endpointFrame{}, false, errEndpointStopped
	}
	if len(w.ready) == 0 {
		var buf [512]byte
		n := 0
		e := w.controlSource(w.in, w.readID, func(fd int) error { var x error; n, x = unix.Read(fd, buf[:]); return x })
		// errors.Join wraps a lone errno; unwrap only exact one constituent, never
		// treat joined custody errors as ordinary transport no-progress.
		if n <= 0 && endpointOnlyErrno(e, unix.EAGAIN, unix.EINTR) {
			return endpointFrame{}, false, nil
		}
		if e != nil || n <= 0 {
			if e == nil {
				e = errors.Join(io.EOF, w.decoder.eof())
			}
			return endpointFrame{}, false, w.fail(e)
		}
		frames, e := w.decoder.feed(buf[:n])
		if e != nil {
			return endpointFrame{}, false, w.fail(e)
		}
		for _, f := range frames {
			if f.direction != w.inbound || f.generation != w.generation || w.received == ^uint64(0) || f.sequence != w.received+1 {
				return endpointFrame{}, false, w.fail(errEndpointFrame)
			}
			w.received = f.sequence
		}
		w.ready, w.inputStopped = endpointTerminalFirst(frames)
	}
	if len(w.ready) == 0 {
		return endpointFrame{}, false, nil
	}
	f := w.ready[0]
	w.ready = w.ready[1:]
	return f, true, nil
}
func endpointOnlyErrno(e error, allowed ...error) bool {
	if e == nil {
		return false
	}
	if list, ok := e.(interface{ Unwrap() []error }); ok {
		xs := list.Unwrap()
		if len(xs) != 1 {
			return false
		}
		return endpointOnlyErrno(xs[0], allowed...)
	}
	for _, a := range allowed {
		if e == a {
			return true
		}
	}
	return false
}
func (w *endpointPipeWire) queueRawSource(raw []byte) error {
	if w == nil || w.ended != nil {
		return errEndpointStopped
	}
	if w.pending != nil {
		return errEndpointBackpressure
	}
	f, e := decodeEndpointFrame(raw)
	if e != nil {
		return w.fail(e)
	}
	if f.direction != 3-w.inbound || f.generation != w.generation || w.sent == ^uint64(0) || f.sequence != w.sent+1 || len(raw) > 4096 {
		return w.fail(errEndpointFrame)
	}
	w.pending = append([]byte(nil), raw...)
	return nil
}
func (w *endpointPipeWire) queueSource(kind byte, p []byte) error {
	raw, e := encodeEndpointFrame(endpointFrame{kind: kind, direction: 3 - w.inbound, generation: w.generation, sequence: w.sent + 1, payload: p})
	if e != nil {
		return e
	}
	return w.queueRawSource(raw)
}
func (w *endpointPipeWire) flushSource() (bool, error) {
	if w == nil || w.ended != nil {
		return false, errEndpointStopped
	}
	if w.pending == nil {
		return false, nil
	}
	n := 0
	e := w.controlSource(w.out, w.writeID, func(fd int) error { var x error; n, x = unix.Write(fd, w.pending); return x })
	// <=PIPE_BUF one writer, O_NONBLOCK: EAGAIN has written no bytes. Only this
	// framed bridge buffer may wait for another turn. No TUN datagram replay.
	if n <= 0 && endpointOnlyErrno(e, unix.EAGAIN, unix.EINTR) {
		return false, nil
	}
	if e != nil || n != len(w.pending) {
		return false, w.fail(errors.Join(e, ErrConflict))
	}
	w.sent++
	w.pending = nil
	return true, nil
}
func (w *endpointPipeWire) waitSource(ctx context.Context, output bool) error {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return ErrConflict
		}
		return ctx.Err()
	}
	d := 25 * time.Millisecond
	if end, ok := ctx.Deadline(); ok && time.Until(end) < d {
		d = time.Until(end)
	}
	if d <= 0 {
		return context.DeadlineExceeded
	}
	f, id, event := w.in, w.readID, int16(unix.POLLIN)
	if output {
		f, id, event = w.out, w.writeID, unix.POLLOUT
	}
	e := w.controlSource(f, id, func(fd int) error {
		p := []unix.PollFd{{Fd: int32(fd), Events: event}}
		ts := unix.NsecToTimespec(d.Nanoseconds())
		_, e := unix.Ppoll(p, &ts, nil)
		if e == unix.EINTR {
			return nil
		}
		if e != nil {
			return e
		}
		if event == unix.POLLIN && p[0].Revents&unix.POLLIN != 0 {
			return nil
		}
		if p[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return io.EOF
		}
		return nil
	})
	if e != nil {
		return w.fail(e)
	}
	return ctx.Err()
}
func (w *endpointPipeWire) closeWriteSource() error {
	if w == nil {
		return nil
	}
	w.writeOnce.Do(func() { w.writeClosed.Store(true); w.writeErr = w.out.Close() })
	return w.writeErr
}
func (w *endpointPipeWire) closeSource() error {
	if w == nil {
		return nil
	}
	w.fail(errEndpointStopped)
	return w.closeErr
}
