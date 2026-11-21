//go:build linux && mnet_enrollment_development

package osownership

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// Fixed client-bootstrap source runs before any ordinary client work. It has
// no callback and cannot return to a cap-bearing runtime after success: exec
// replaces ALL existing threads. Subsequent runtime threads inherit zero caps.
// No bootstrap handler is wired into an executable; guard is still false.
func execUnprivilegedClientSource() error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	parent, e := strconv.Atoi(os.Getenv("MNO_CLIENT_PARENT_PID"))
	if e != nil || parent <= 1 || os.Getppid() != parent {
		return ErrConflict
	}
	if e = unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); e != nil {
		return e
	}
	if e = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); e != nil {
		return e
	}
	lastBytes, e := os.ReadFile("/proc/sys/kernel/cap_last_cap")
	if e != nil {
		return e
	}
	last, e := strconv.Atoi(strings.TrimSpace(string(lastBytes)))
	if e != nil || last < 0 || last > 63 {
		return ErrConflict
	}
	for cap := 0; cap <= last; cap++ {
		if e = unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(cap), 0, 0, 0); e != nil {
			return e
		}
	}
	// SECBIT_NOROOT|NOROOT_LOCKED|NO_SETUID_FIXUP|NO_SETUID_FIXUP_LOCKED|
	// KEEP_CAPS_LOCKED: uid0 alone cannot regain capabilities at exec.
	if e = unix.Prctl(unix.PR_SET_SECUREBITS, 1|2|4|8|32, 0, 0, 0); e != nil {
		return e
	}
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	data := [2]unix.CapUserData{}
	if e = unix.Capset(&hdr, &data[0]); e != nil {
		return e
	}
	if e = unix.CloseRange(3, ^uint(0), unix.CLOSE_RANGE_CLOEXEC); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if os.Getppid() != parent {
		return ErrConflict
	}
	return syscall.Exec(exe, []string{exe, "--osownership-client-unprivileged-source"}, []string{"PATH=/usr/bin:/bin", "MNO_CLIENT_PARENT_PID=" + strconv.Itoa(parent)})
}

// Must pass in final unprivileged entry BEFORE client sockets or app work.
func verifyClientSandboxSource() error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	b, e := os.ReadFile("/proc/self/status")
	if e != nil {
		return e
	}
	if e = validateClientStatus(string(b)); e != nil {
		return e
	}
	fds, e := os.ReadDir("/proc/self/fd")
	if e != nil {
		return e
	}
	for _, fd := range fds {
		n, e := strconv.Atoi(fd.Name())
		if e != nil {
			return e
		}
		if n <= 2 {
			continue
		}
		if _, e = os.Readlink("/proc/self/fd/" + fd.Name()); os.IsNotExist(e) {
			continue
		}
		return errors.New("client inherited descriptor outside stdin/out/err allowlist")
	}
	if os.Getppid() <= 1 {
		return ErrConflict
	}
	return nil
}
func validateClientStatus(status string) error {
	fields := map[string]string{}
	for _, l := range strings.Split(status, "\n") {
		kv := strings.SplitN(l, ":", 2)
		if len(kv) == 2 {
			fields[kv[0]] = strings.TrimSpace(kv[1])
		}
	}
	for _, k := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
		v, ok := fields[k]
		if !ok {
			return ErrConflict
		}
		n, e := strconv.ParseUint(v, 16, 64)
		if e != nil || n != 0 {
			return errors.New("client capabilities not zero")
		}
	}
	if fields["NoNewPrivs"] != "1" {
		return errors.New("client no-new-privs absent")
	}
	return nil
}
