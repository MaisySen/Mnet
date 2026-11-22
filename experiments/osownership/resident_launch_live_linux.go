//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func spawnResidentMode(ctx context.Context, mode string) (ResidentProcess, error) {
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
		p, e := spawnResidentPinnedMode(ctx, mode)
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
func spawnResidentPinnedSource(ctx context.Context) (ResidentProcess, error) {
	return spawnResidentPinnedMode(ctx, "--osownership-resident-no-tun-source")
}
func spawnResidentPinnedMode(ctx context.Context, mode string) (ResidentProcess, error) {
	return spawnResidentPinnedModeFiles(ctx, mode, nil)
}
func spawnResidentPinnedModeFiles(ctx context.Context, mode string, extra []*os.File) (process ResidentProcess, retErr error) {
	if !allowedResidentEvidenceMode(mode) || (mode == endpointResidentMode && len(extra) != 2) || (mode != endpointResidentMode && len(extra) != 0) {
		return nil, ErrExecutorUnreleased
	}
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
	cmd.Args[1] = mode
	if mode == endpointResidentMode {
		cmd.ExtraFiles = append(cmd.ExtraFiles, extra...)
		ticks, e := readProcessStartTicks(os.Getpid())
		if e != nil {
			r.Close()
			w.Close()
			return nil, e
		}
		cmd.Env = append(cmd.Env, "MNO_ENDPOINT_PARENT_TICKS="+strconv.FormatUint(ticks, 10))
		for i, f := range extra {
			var st unix.Stat_t
			if e := unix.Fstat(int(f.Fd()), &st); e != nil {
				r.Close()
				w.Close()
				return nil, e
			}
			prefix := "MNO_ENDPOINT_" + strconv.Itoa(i)
			cmd.Env = append(cmd.Env, prefix+"_DEV="+strconv.FormatUint(uint64(st.Dev), 10), prefix+"_INO="+strconv.FormatUint(st.Ino, 10))
		}
	}
	if e = cmd.Start(); e != nil {
		r.Close()
		w.Close()
		return nil, e
	}
	w.Close()
	gateR.Close()
	pidfd, e := unix.PidfdOpen(cmd.Process.Pid, 0)
	ticks, identityErr := readProcessStartTicks(cmd.Process.Pid)
	wait := make(chan struct{})
	c := &residentChild{cmd: cmd, pidfd: pidfd, reader: r, wait: wait, startTicks: ticks, identityObserved: identityErr == nil}
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
	if identityErr != nil {
		return nil, identityErr
	}
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

func unitTestLiveLaunchBuildConflict() {}
