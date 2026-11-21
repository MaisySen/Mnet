//go:build linux && mnet_enrollment_development

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
	cmd        *exec.Cmd
	pidfd      int
	nsfd       *os.File
	reader     *os.File
	ns         Namespace
	wait       chan struct{}
	waitErr    error
	readerOnce sync.Once
	closeOnce  sync.Once
	closeErr   error
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
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return nil, ErrExecutorUnreleased
	}
	type answer struct {
		p ResidentProcess
		e error
	}
	ch := make(chan answer, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		p, e := spawnResidentPinnedSource(ctx)
		ch <- answer{p, e}
		if child, ok := p.(*residentChild); ok {
			<-child.wait
		} else {
			var pending *PendingResidentError
			if errors.As(e, &pending) {
				if child, ok := pending.Process.(*residentChild); ok {
					<-child.wait
				}
			}
		}
	}()
	r := <-ch
	return r.p, r.e
}
func spawnResidentPinnedSource(ctx context.Context) (process ResidentProcess, retErr error) {
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return nil, ErrExecutorUnreleased
	}
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	r, w, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	gateR, gateW, e := os.Pipe()
	if e != nil {
		r.Close()
		w.Close()
		return nil, e
	}
	defer gateR.Close()
	defer gateW.Close()
	parentNS, e := os.Open("/proc/thread-self/ns/net")
	if e != nil {
		r.Close()
		w.Close()
		return nil, e
	}
	defer parentNS.Close()
	var outerStat unix.Stat_t
	if e = unix.Fstat(int(parentNS.Fd()), &outerStat); e != nil {
		r.Close()
		w.Close()
		return nil, e
	}
	outerIdentity := Namespace{Device: uint64(outerStat.Dev), Inode: outerStat.Ino}
	cmd := residentCommand(exe, os.Getuid(), os.Getgid(), w, outerIdentity, os.Getpid(), gateR)
	if e = cmd.Start(); e != nil {
		r.Close()
		w.Close()
		return nil, e
	}
	w.Close()
	gateR.Close()
	pidfd, e := unix.PidfdOpen(cmd.Process.Pid, 0)
	wait := make(chan struct{})
	c := &residentChild{cmd: cmd, pidfd: pidfd, reader: r, wait: wait}
	// Child PID cannot be reused before this reaper starts: unreaped zombie
	// keeps its PID. Pidfd acquisition is deliberately above cmd.Wait.
	go func() { c.waitErr = cmd.Wait(); close(wait) }()
	if e != nil {
		c.pidfd = -1
		cleanup, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if stopErr := c.AbortAndReap(cleanup); stopErr != nil {
			return nil, errors.Join(e, &PendingResidentError{Process: c, Cause: stopErr})
		}
		_ = c.CloseCustody()
		return nil, e
	}
	c.pidfd = pidfd
	ok := false
	defer func() {
		if !ok {
			cleanup, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			if e := c.AbortAndReap(cleanup); e != nil {
				retErr = errors.Join(retErr, &PendingResidentError{Process: c, Cause: e})
				return
			}
			if c.nsfd != nil {
				retErr = errors.Join(retErr, c.CloseCustody())
			} else {
				r.Close()
				unix.Close(pidfd)
			}
		}
	}()
	if e = pidfdAlive(pidfd); e != nil {
		return nil, e
	}
	// /proc path is rooted in the just-created, pidfd-held child lifetime. The
	// re-exec child must block on startup rendezvous; no handler is wired yet.
	f, e := os.OpenFile("/proc/"+strconv.Itoa(cmd.Process.Pid)+"/ns/net", os.O_RDONLY, 0)
	if e != nil {
		return nil, e
	}
	c.nsfd = f
	var st unix.Stat_t
	if e = unix.Fstat(int(f.Fd()), &st); e != nil {
		return nil, e
	}
	var fs unix.Statfs_t
	if e = unix.Fstatfs(int(f.Fd()), &fs); e != nil || fs.Type != unix.NSFS_MAGIC {
		return nil, ErrConflict
	}
	typ, e := unix.IoctlRetInt(int(f.Fd()), unix.NS_GET_NSTYPE)
	if e != nil || typ != unix.CLONE_NEWNET {
		return nil, ErrConflict
	}
	parent, e := os.Open("/proc/thread-self/ns/net")
	if e != nil {
		return nil, e
	}
	defer parent.Close()
	var outer unix.Stat_t
	if e = unix.Fstat(int(parent.Fd()), &outer); e != nil || outer.Dev == st.Dev && outer.Ino == st.Ino {
		return nil, ErrConflict
	}
	if e = proveChildUserNamespace(cmd.Process.Pid, int(f.Fd()), pidfd); e != nil {
		return nil, e
	}
	c.ns = Namespace{Device: uint64(st.Dev), Inode: st.Ino, Isolated: true}
	if _, e = gateW.Write([]byte{1}); e != nil {
		return nil, e
	}
	gateW.Close()
	ok = true
	return c, nil
}

// Client is re-execed by the resident owner, not the outer parent. No clone or
// setns: same namespace by exec inheritance, no TUN ExtraFiles, CLOEXEC custody.
func startResidentClientSource(ctx context.Context, ns *namespaceCustody) (*exec.Cmd, error) {
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return nil, ErrExecutorUnreleased
	}
	if ns == nil {
		return nil, ErrConflict
	}
	if e := ns.checkCurrent(); e != nil {
		return nil, e
	}
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	c := exec.CommandContext(ctx, exe, "--osownership-client-capdrop-bootstrap-source")
	c.Env = []string{"PATH=/usr/bin:/bin", "MNO_CLIENT_PARENT_PID=" + strconv.Itoa(os.Getpid())}
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	return c, c.Start()
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

func residentCommand(exe string, uid, gid int, verdict *os.File, parent Namespace, parentPID int, gate *os.File) *exec.Cmd {
	cmd := exec.Command(exe, "--osownership-resident-no-tun-source")
	cmd.Stdin = gate
	cmd.Env = []string{"PATH=/usr/bin:/bin", "MNO_PARENT_DEV=" + strconv.FormatUint(parent.Device, 10), "MNO_PARENT_INO=" + strconv.FormatUint(parent.Inode, 10), "MNO_PARENT_PID=" + strconv.Itoa(parentPID)}
	cmd.ExtraFiles = []*os.File{verdict}
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWUSER | unix.CLONE_NEWNET, UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: uid, Size: 1}}, GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: gid, Size: 1}}, GidMappingsEnableSetgroups: false, Pdeathsig: syscall.SIGKILL}
	return cmd
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
