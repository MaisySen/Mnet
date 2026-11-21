//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func execUnprivilegedClientSource() error {
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return ErrExecutorUnreleased
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := emitEvidencePhase("bootstrap-entry"); e != nil {
		return e
	}
	parent, e := strconv.Atoi(os.Getenv("MNO_CLIENT_PARENT_PID"))
	if e != nil || parent <= 1 || os.Getppid() != parent {
		return ErrConflict
	}
	if e = unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); e != nil {
		return opFailure("no-new-privs", "runner", e)
	}
	if e = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); e != nil {
		return opFailure("ambient-clear", "runner", e)
	}
	lastBytes, e := os.ReadFile("/proc/sys/kernel/cap_last_cap")
	if e != nil {
		return opFailure("cap-last-read", "runner", e)
	}
	last, e := strconv.Atoi(strings.TrimSpace(string(lastBytes)))
	if e != nil || last < 0 || last > 63 {
		return opFailure("cap-last-read", "runner", ErrConflict)
	}
	for cap := 0; cap <= last; cap++ {
		if e = unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(cap), 0, 0, 0); e != nil {
			return opFailure("bounding-drop", "runner", e)
		}
	}
	// SECBIT_NOROOT|NOROOT_LOCKED|NO_SETUID_FIXUP|NO_SETUID_FIXUP_LOCKED|
	// KEEP_CAPS_LOCKED: uid0 alone cannot regain capabilities at exec.
	if e = unix.Prctl(unix.PR_SET_SECUREBITS, clientSecurebits, 0, 0, 0); e != nil {
		return opFailure("securebits-lock", "runner", e)
	}
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	data := [2]unix.CapUserData{}
	if e = unix.Capset(&hdr, &data[0]); e != nil {
		return opFailure("capset-zero", "runner", e)
	}
	if e := emitEvidencePhase("capset-zero"); e != nil {
		return e
	}
	filterBaseline, e := currentSeccompFilterCount()
	if e != nil {
		return opFailure("filter-baseline", "runner", e)
	}
	if e = installEvidenceClientFilter(os.Getpid()); e != nil {
		return opFailure("filter-install", "runner", e)
	}
	if e := emitEvidencePhase("filter-installed"); e != nil {
		return e
	}
	if e = unix.CloseRange(3, ^uint(0), unix.CLOSE_RANGE_CLOEXEC); e != nil {
		return opFailure("descriptor-cloexec", "runner", e)
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if os.Getppid() != parent {
		return ErrConflict
	}
	return opFailure("client-exec", "runner", syscall.Exec(exe, []string{exe, "--osownership-client-unprivileged-source"}, []string{"PATH=/usr/bin:/bin", "MNO_CLIENT_PARENT_PID=" + strconv.Itoa(parent), "MNO_CLIENT_FILTER_BASELINE=" + strconv.Itoa(filterBaseline)}))
}
