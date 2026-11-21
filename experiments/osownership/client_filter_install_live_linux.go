//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"golang.org/x/sys/unix"
	"unsafe"
)

func installEvidenceClientFilter(pid int) error {
	f, e := evidenceClientFilter(pid)
	if e != nil {
		return e
	}
	p := unix.SockFprog{Len: uint16(len(f)), Filter: &f[0]}
	return unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&p)), 0, 0)
}
