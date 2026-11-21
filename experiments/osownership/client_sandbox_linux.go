//go:build linux && mnet_enrollment_development

package osownership

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Fixed client-bootstrap source runs before any ordinary client work. It has
// no callback and cannot return to a cap-bearing runtime after success: exec
// replaces ALL existing threads. Subsequent runtime threads inherit zero caps.
// No bootstrap handler is wired into an executable; guard is still false.
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

// Must pass in final unprivileged entry BEFORE client sockets or app work.
func verifyClientSandboxSource() error {
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return ErrExecutorUnreleased
	}
	fds, e := clientFDInventorySource()
	if e != nil {
		return e
	}
	if e = validateFDAllowlist(fds); e != nil {
		return e
	}
	b, e := rawReadBounded("/proc/self/status", 16384)
	if e != nil {
		return e
	}
	if e = validateClientStatus(string(b)); e != nil {
		return e
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
	if fields["Seccomp"] != "2" {
		return errors.New("client seccomp absent")
	}
	if fields["NoNewPrivs"] != "1" {
		return errors.New("client no-new-privs absent")
	}
	return nil
}

// Strict pre-app inventory uses no os.File/poll registration. The only
// exclusion is the exact directory fd opened here under our own custody.
// Any preexisting runtime poll fd is NOT silently allowed: harness failure.
func clientFDInventorySource() ([]int, error) {
	fd, e := unix.Open("/proc/self/fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(fd)
	buf := make([]byte, 8192)
	var names []string
	total := 0
	for {
		n, e := unix.Getdents(fd, buf)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return nil, e
		}
		if n == 0 {
			break
		}
		total += n
		if total > 65536 {
			return nil, errors.New("fd inventory cap")
		}
		used, _, next := unix.ParseDirent(buf[:n], -1, nil)
		if used != n {
			return nil, errors.New("invalid dirent framing")
		}
		names = append(names, next...)
	}
	return parseOwnedFDInventory(names, fd)
}
func parseOwnedFDInventory(names []string, ownedDirectoryFD int) ([]int, error) {
	if ownedDirectoryFD < 0 {
		return nil, ErrConflict
	}
	var out []int
	excluded := false
	seen := map[int]bool{}
	for _, name := range names {
		if name == "." || name == ".." {
			continue
		}
		n, e := strconv.Atoi(name)
		if e != nil || n < 0 || seen[n] {
			return nil, ErrConflict
		}
		seen[n] = true
		if n == ownedDirectoryFD {
			excluded = true
			continue
		}
		out = append(out, n)
	}
	if !excluded {
		return nil, errors.New("owned inventory fd absent")
	}
	sort.Ints(out)
	return out, nil
}
func rawReadBounded(path string, limit int) ([]byte, error) {
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(fd)
	buf := make([]byte, limit+1)
	total := 0
	for {
		n, e := unix.Read(fd, buf[total:])
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return nil, e
		}
		total += n
		if total > limit {
			return nil, errors.New("raw proc read cap")
		}
		if n == 0 {
			return buf[:total], nil
		}
	}
}
func validateFDAllowlist(fds []int) error {
	seen := map[int]bool{}
	for _, n := range fds {
		if n < 0 || n > 2 || seen[n] {
			return &FDInventoryHarnessError{}
		}
		seen[n] = true
	}
	return nil
}

func currentSeccompFilterCount() (int, error) {
	b, e := os.ReadFile("/proc/self/status")
	if e != nil {
		return 0, e
	}
	for _, l := range strings.Split(string(b), "\n") {
		kv := strings.SplitN(l, ":", 2)
		if len(kv) == 2 && kv[0] == "Seccomp_filters" {
			n, e := strconv.Atoi(strings.TrimSpace(kv[1]))
			if e != nil || n < 0 {
				return 0, ErrConflict
			}
			return n, nil
		}
	}
	return 0, errors.New("seccomp filter count unavailable")
}

type FDInventoryHarnessError struct{}

func (*FDInventoryHarnessError) Error() string {
	return "harness: strict pre-app FD inventory has unexpected descriptor; ownership unverified"
}

// Fixed source-selected phases only, raw stderr write avoids os.File poll init.
func emitEvidencePhase(phase string) error {
	if !readOnlyEvidenceBuild {
		return nil
	}
	if !allowedPhase(phase) {
		return ErrConflict
	}
	b := append(mustJSON(phaseRecord{Version: 1, Phase: phase}), '\n')
	for len(b) > 0 {
		n, e := unix.Write(2, b)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return e
		}
		if n <= 0 {
			return ErrConflict
		}
		b = b[n:]
	}
	return nil
}
