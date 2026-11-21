//go:build linux && mnet_enrollment_development

package osownership

// These source-only syscall entry points are mechanically unreleased. No caller
// can construct a release. No unit invokes them. They do NOT implement Kernel.
import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

var ErrExecutorUnreleased = errors.New("OS executor not released")

type namespaceCustody struct {
	fd       int
	identity Namespace
	threadID int
}

const kernelExecutionReleased = false

// proveNamespaceFD duplicates caller-owned FD, checks nsfs and network-namespace
// type, compares original parent/current target. Does not setns or open a path
// supplied by the caller. Exclusive ancestry/userns containment still needs
// supervisor proof before this check can ever be enabled.
func proveNamespaceFD(targetFD, parentFD int) (*namespaceCustody, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	target, e := unix.FcntlInt(uintptr(targetFD), unix.F_DUPFD_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			unix.Close(target)
		}
	}()
	inspect := func(fd int) (Namespace, error) {
		var s unix.Stat_t
		var fs unix.Statfs_t
		if e := unix.Fstat(fd, &s); e != nil {
			return Namespace{}, e
		}
		if e := unix.Fstatfs(fd, &fs); e != nil {
			return Namespace{}, e
		}
		if fs.Type != unix.NSFS_MAGIC {
			return Namespace{}, errors.New("not nsfs")
		}
		kind, e := unix.IoctlRetInt(fd, unix.NS_GET_NSTYPE)
		if e != nil || kind != unix.CLONE_NEWNET {
			return Namespace{}, errors.New("not network namespace")
		}
		return Namespace{Device: uint64(s.Dev), Inode: s.Ino, Isolated: true}, nil
	}
	id, e := inspect(target)
	if e != nil {
		return nil, e
	}
	parentCopy, e := unix.FcntlInt(uintptr(parentFD), unix.F_DUPFD_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(parentCopy)
	parent, e := inspect(parentCopy)
	if e != nil || id == parent {
		return nil, errors.New("target is initial parent namespace or parent invalid")
	}
	current, e := unix.Open("/proc/thread-self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(current)
	here, e := inspect(current)
	if e != nil || here != id {
		return nil, errors.New("current namespace does not match held target FD")
	}
	ok = true
	return &namespaceCustody{fd: target, identity: id, threadID: unix.Gettid()}, nil
}

// openExclusiveNonpersistentTUN cannot adopt an existing TUN: IFF_TUN_EXCL and
// no persistence flag. FD_CLOEXEC is set at open and verified. Alias/address/
// routes are NOT configured here. Future executor must ACK/read back those
// separately and hold namespaceCustody until cleanup is verified.
func openExclusiveNonpersistentTUN(ns *namespaceCustody, name string) (*os.File, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ns == nil || ns.fd < 0 {
		return nil, errors.New("held namespace custody required")
	}
	var st unix.Stat_t
	if e := unix.Fstat(ns.fd, &st); e != nil || uint64(st.Dev) != ns.identity.Device || st.Ino != ns.identity.Inode {
		return nil, ErrConflict
	}
	current, e := unix.Open("/proc/thread-self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(current)
	var here unix.Stat_t
	if e = unix.Fstat(current, &here); e != nil || uint64(here.Dev) != ns.identity.Device || here.Ino != ns.identity.Inode {
		return nil, ErrConflict
	}
	if len(name) < 7 || len(name) > 15 {
		return nil, errors.New("dedicated TUN name required")
	}
	if e := ns.checkCurrent(); e != nil {
		return nil, e
	}
	fd, e := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			unix.Close(fd)
		}
	}()
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if e != nil || flags&unix.FD_CLOEXEC == 0 {
		return nil, errors.New("CLOEXEC missing")
	}
	ifr, e := unix.NewIfreq(name)
	if e != nil {
		return nil, e
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI | unix.IFF_TUN_EXCL)
	if e = ns.checkCurrent(); e != nil {
		return nil, e
	}
	if e = unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); e != nil {
		return nil, e
	}
	if ifr.Name() != name {
		return nil, errors.New("unexpected kernel interface name")
	}
	ok = true
	return os.NewFile(uintptr(fd), "owned-nonpersistent-tun"), nil
}
