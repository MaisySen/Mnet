//go:build linux && mnet_enrollment_development

package osownership

import (
	"errors"
	"golang.org/x/sys/unix"
	"runtime"
)

// Fixed read-only evidence client barrier, not a general hostile-app sandbox.
// Deny signal targets other than self-tgid SIGURG tgkill used by Go runtime.
// x86_64 only: x32 and other arch fail. Filter is installed before fixed exec,
// inherited across exec and then every client thread. No filter run in tests.
func evidenceClientFilter(pid int) ([]unix.SockFilter, error) {
	if !evidenceRuntimeSupported(runtime.GOOS, runtime.GOARCH, runtime.Version()) || pid <= 1 {
		return nil, errors.New("unsupported evidence client architecture/PID")
	}
	const ld = 0x20
	const jeq = 0x15
	const ret = 0x06
	deny := uint32(unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM))
	allow := uint32(unix.SECCOMP_RET_ALLOW)
	kill := uint32(unix.SECCOMP_RET_KILL_PROCESS)
	f := []unix.SockFilter{{Code: ld, K: 4}, {Code: jeq, K: unix.AUDIT_ARCH_X86_64, Jt: 1}, {Code: ret, K: kill}, {Code: ld, K: 0}, {Code: 0x35, K: 0x40000000, Jf: 1}, {Code: ret, K: kill}}
	for _, nr := range []uint32{unix.SYS_KILL, unix.SYS_TKILL, unix.SYS_RT_SIGQUEUEINFO, unix.SYS_RT_TGSIGQUEUEINFO, unix.SYS_PIDFD_SEND_SIGNAL, unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PIDFD_GETFD, unix.SYS_SETNS, unix.SYS_UNSHARE, unix.SYS_CLONE3, unix.SYS_IO_URING_SETUP} {
		f = append(f, unix.SockFilter{Code: jeq, K: nr, Jf: 1}, unix.SockFilter{Code: ret, K: deny})
	}
	// clone syscall: allow only ordinary Go thread clone flags; no new namespace,
	// ptrace, parent/exit-signal or arbitrary fork shape. Go source owns this
	// exact clone mask INCLUDING amd64 assembly CLONE_SETTLS. Nil-mp/gp base
	// 0x50f00 is deliberately refused. Unsupported runtime shape fails closed.
	f = append(f, unix.SockFilter{Code: jeq, K: unix.SYS_CLONE, Jf: 8}, unix.SockFilter{Code: ld, K: 20}, unix.SockFilter{Code: jeq, K: 0, Jf: 4}, unix.SockFilter{Code: ld, K: 16}, unix.SockFilter{Code: jeq, K: 0xd0f00, Jf: 1}, unix.SockFilter{Code: ret, K: allow}, unix.SockFilter{Code: ret, K: deny}, unix.SockFilter{Code: ret, K: deny}, unix.SockFilter{Code: ret, K: deny})
	f = append(f, unix.SockFilter{Code: jeq, K: unix.SYS_TGKILL, Jf: 8}, unix.SockFilter{Code: ld, K: 20}, unix.SockFilter{Code: jeq, K: 0, Jf: 5}, unix.SockFilter{Code: ld, K: 16}, unix.SockFilter{Code: jeq, K: uint32(pid), Jf: 3}, unix.SockFilter{Code: ld, K: 32}, unix.SockFilter{Code: jeq, K: uint32(unix.SIGURG), Jf: 1}, unix.SockFilter{Code: ret, K: allow}, unix.SockFilter{Code: ret, K: deny}, unix.SockFilter{Code: ret, K: allow})
	return f, nil
}

const clientSecurebits = 1 | 2 | 4 | 8 | 32

// Version upgrades require fresh source+assembly characterization and review.
func evidenceRuntimeSupported(goos, arch, version string) bool {
	return goos == "linux" && arch == "amd64" && version == "go1.26.6"
}
