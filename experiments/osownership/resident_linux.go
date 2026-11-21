//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Resident source has no setns/unshare calls. A fresh exec is born in NEWUSER|
// NEWNET and never enters another namespace. Namespace operations additionally
// run on one locked thread and custody checks reject any TID mismatch.
func withResidentNamespace(target int, parent Namespace, fn func(*namespaceCustody) error) error {
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return ErrExecutorUnreleased
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ns, e := proveNamespaceFD(target, parent)
	if e != nil {
		return e
	}
	defer unix.Close(ns.fd)
	return fn(ns)
}

type residentSpawner struct{}
type residentChild struct {
	cmd              *exec.Cmd
	pidfd            int
	nsfd             *os.File
	reader           *os.File
	ns               Namespace
	wait             chan struct{}
	waitErr          error
	startTicks       uint64
	identityObserved bool
	readerOnce       sync.Once
	closeOnce        sync.Once
	closeErr         error
}

func (c *residentChild) Namespace() Namespace     { return c.ns }
func (c *residentChild) VerdictReader() io.Reader { return c.reader }
func (c *residentChild) Reap(ctx context.Context) error {
	select {
	case <-c.wait:
		return c.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *residentChild) AbortAndReap(ctx context.Context) error {
	c.readerOnce.Do(func() { _ = c.reader.Close() })
	var e error
	if c.pidfd >= 0 {
		e = unix.PidfdSendSignal(c.pidfd, unix.SIGKILL, nil, 0)
	} else {
		e = c.cmd.Process.Kill()
	}
	if e == unix.ESRCH {
		e = nil
	}
	reapErr := c.Reap(ctx)
	var exited *exec.ExitError
	if errors.As(reapErr, &exited) {
		reapErr = nil
	}
	return errors.Join(e, reapErr)
}
func (c *residentChild) CloseCustody() error {
	c.closeOnce.Do(func() {
		c.readerOnce.Do(func() { c.closeErr = c.reader.Close() })
		if c.nsfd != nil {
			c.closeErr = errors.Join(c.closeErr, c.nsfd.Close())
		}
		if c.pidfd >= 0 {
			c.closeErr = errors.Join(c.closeErr, unix.Close(c.pidfd))
		}
	})
	return c.closeErr
}
func (residentSpawner) Spawn(ctx context.Context) (ResidentProcess, error) {
	return spawnResidentMode(ctx, "--osownership-resident-no-tun-source")
}

// Resident bootstrap is source only, NOT wired into any app entry point. The
// child waits on stdin while outer verifies its nsfs/pidfd custody. Parent-death
// SIGKILL is configured by the guarded launcher; no signal was executed.
func residentBootstrapSource(ctx context.Context, work func(context.Context, *namespaceCustody) (Verdict, error)) error {
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return ErrExecutorUnreleased
	}
	unix.CloseOnExec(3)
	unix.CloseOnExec(0)
	if os.Getppid() != parseSourceParentPID() {
		return ErrConflict
	}
	var start [1]byte
	if _, e := io.ReadFull(os.Stdin, start[:]); e != nil || start[0] != 1 {
		return ErrConflict
	}
	// Parent has acquired its nsfd before releasing gate. Becoming
	// nondumpable earlier would deny that /proc namespace custody access.
	if e := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); e != nil {
		return e
	}
	self, e := os.Open("/proc/thread-self/ns/net")
	if e != nil {
		return e
	}
	defer self.Close()
	pipe := os.NewFile(3, "bounded-resident-verdict")
	if pipe == nil {
		return ErrConflict
	}
	defer pipe.Close()
	bounded, cancel := context.WithTimeout(ctx, 4500*time.Millisecond)
	defer cancel()
	parentIdentity, e := parseSourceParentNamespace()
	if e != nil {
		return e
	}
	if os.Getppid() != parseSourceParentPID() {
		return ErrConflict
	}
	return withResidentNamespace(int(self.Fd()), parentIdentity, func(ns *namespaceCustody) error {
		v, workErr := work(bounded, ns)
		if workErr != nil {
			v.Version = 1
			v.Sequence = 1
			v.Namespace = ns.identity
			v.Result = "fail"
			if v.FailureClass == "" {
				v.FailureClass = "runner"
			}
		}
		b, e := json.Marshal(v)
		if e != nil || len(b) > maxVerdictBytes {
			return errors.New("invalid bounded verdict")
		}
		_, e = pipe.Write(append(b, '\n'))
		return errors.Join(workErr, e)
	})
}

func pidfdAlive(fd int) error {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, e := unix.Poll(fds, 0)
	if e != nil {
		return e
	}
	if n != 0 || fds[0].Revents != 0 {
		return errors.New("child lifetime ended")
	}
	return nil
}
func proveChildUserNamespace(pid, netfd, pidfd int) error {
	if e := pidfdAlive(pidfd); e != nil {
		return e
	}
	owner, e := unix.IoctlRetInt(netfd, unix.NS_GET_USERNS)
	if e != nil {
		return e
	}
	defer unix.Close(owner)
	child, e := os.Open("/proc/" + strconv.Itoa(pid) + "/ns/user")
	if e != nil {
		return e
	}
	defer child.Close()
	outer, e := os.Open("/proc/thread-self/ns/user")
	if e != nil {
		return e
	}
	defer outer.Close()
	var a, b, c unix.Stat_t
	if unix.Fstat(owner, &a) != nil || unix.Fstat(int(child.Fd()), &b) != nil || unix.Fstat(int(outer.Fd()), &c) != nil {
		return ErrConflict
	}
	if a.Dev != b.Dev || a.Ino != b.Ino || a.Dev == c.Dev && a.Ino == c.Ino {
		return ErrConflict
	}
	return pidfdAlive(pidfd)
}

func parseSourceParentPID() int {
	v, e := strconv.Atoi(os.Getenv("MNO_PARENT_PID"))
	if e != nil || v <= 1 {
		return -1
	}
	return v
}
func parseSourceParentNamespace() (Namespace, error) {
	d, e := strconv.ParseUint(os.Getenv("MNO_PARENT_DEV"), 10, 64)
	if e != nil {
		return Namespace{}, e
	}
	i, e := strconv.ParseUint(os.Getenv("MNO_PARENT_INO"), 10, 64)
	if e != nil || d == 0 || i == 0 {
		return Namespace{}, ErrConflict
	}
	return Namespace{Device: d, Inode: i}, nil
}

func (c *residentChild) LifecycleSnapshot() LifecycleEvidence {
	life := LifecycleEvidence{PID: c.cmd.Process.Pid, StartTicks: c.startTicks, IdentityObserved: c.identityObserved}
	select {
	case <-c.wait:
		life.Reaped = true
		life.ExitCode = c.cmd.ProcessState.ExitCode()
		if status, ok := c.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			life.Signal = int(status.Signal())
		}
	default:
	}
	return life
}
