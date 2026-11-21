//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
)

// SOURCE PROPOSAL ONLY. This performs real read-only FD/ioctl operations if a
// future explicit release permits it. No caller/test executes it. Callers hold
// original fileCustody.mu against close for the complete observation/use interval.
func (o *originalHeldPacketResources) verifyOriginalDeviceLockedSource(ctx context.Context) (*originalDeviceEvidence, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || o == nil || o.creation == nil || o.creation.fd == nil || o.creation.fd.closed || o.creation.fd.file == nil || o.namespace == nil || o.ledger == nil {
		return nil, ErrConflict
	}
	if e := o.namespace.checkCurrent(); e != nil {
		return nil, e
	}
	raw, e := o.creation.fd.file.SyscallConn()
	if e != nil {
		return nil, e
	}
	var evidence *originalDeviceEvidence
	var observedErr error
	controlErr := raw.Control(func(number uintptr) {
		before, e := unix.FcntlInt(number, unix.F_GETFL, 0)
		if e != nil {
			observedErr = e
			return
		}
		evidence, observedErr = o.inspectOriginalControlSource(ctx, int(number))
		after, e := unix.FcntlInt(number, unix.F_GETFL, 0)
		observedErr = errors.Join(observedErr, e)
		if e == nil && after != before {
			observedErr = errors.Join(observedErr, ErrConflict)
		}
	})
	if e = errors.Join(controlErr, observedErr); e != nil {
		return nil, e
	}
	return evidence, nil
}

// Descriptor is valid only inside SyscallConn.Control's callback. Never saved,
// returned, closed, duped, or used after the callback. No F_SETFL/Fd conversion.
func (o *originalHeldPacketResources) inspectOriginalControlSource(ctx context.Context, fd int) (result *originalDeviceEvidence, err error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrConflict
	}
	var device unix.Stat_t
	if e := unix.Fstat(fd, &device); e != nil {
		return nil, e
	}
	if device.Mode&unix.S_IFMT != unix.S_IFCHR || unix.Major(uint64(device.Rdev)) != 10 || unix.Minor(uint64(device.Rdev)) != 200 {
		return nil, ErrConflict
	}
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if e != nil || flags&unix.FD_CLOEXEC == 0 {
		return nil, ErrConflict
	}
	// TUNGETIFF name is correlated with the exact later rtnetlink record. Its
	// NOFILTER/NO_PI aliased bit is NOT used as framing proof.
	ifr, e := unix.NewIfreq("")
	if e != nil {
		return nil, e
	}
	if e = unix.IoctlIfreq(fd, unix.TUNGETIFF, ifr); e != nil {
		return nil, e
	}
	if ifr.Name() != o.creation.handle.Link.Name {
		return nil, ErrConflict
	}
	nsfd, e := unix.IoctlRetInt(fd, unix.TUNGETDEVNETNS)
	if e != nil {
		return nil, e
	}
	defer func() { err = errors.Join(err, unix.Close(nsfd)) }()
	var stat unix.Stat_t
	var fs unix.Statfs_t
	if e = unix.Fstat(nsfd, &stat); e != nil {
		return nil, e
	}
	if e = unix.Fstatfs(nsfd, &fs); e != nil {
		return nil, e
	}
	kind, e := unix.IoctlRetInt(nsfd, unix.NS_GET_NSTYPE)
	if e != nil || fs.Type != unix.NSFS_MAGIC || kind != unix.CLONE_NEWNET {
		return nil, ErrConflict
	}
	observed := Namespace{Device: uint64(stat.Dev), Inode: stat.Ino, Isolated: true}
	if !sameNamespace(observed, o.namespace.identity) {
		return nil, ErrConflict
	}
	if e = o.namespace.checkCurrent(); e != nil {
		return nil, e
	}
	id := o.ledger.snapshot()
	if id.closed || id.target != observed || id.device != observed {
		return nil, ErrConflict
	}
	return &originalDeviceEvidence{resources: o, identity: id, deviceNamespace: observed, tunName: ifr.Name()}, nil
}
