//go:build linux && mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"golang.org/x/sys/unix"
	"strings"
	"testing"
)

func evalEvidenceFilter(t *testing.T, nr, arch uint32, args [6]uint64) uint32 {
	t.Helper()
	f, e := evidenceClientFilter(1234)
	if e != nil {
		t.Skip(e)
	}
	d := make([]byte, 64)
	binary.LittleEndian.PutUint32(d, nr)
	binary.LittleEndian.PutUint32(d[4:], arch)
	for j, v := range args {
		binary.LittleEndian.PutUint64(d[16+j*8:], v)
	}
	var a uint32
	for pc := 0; pc < len(f); pc++ {
		i := f[pc]
		switch i.Code {
		case 0x20:
			a = binary.LittleEndian.Uint32(d[i.K:])
		case 0x15:
			if a == i.K {
				pc += int(i.Jt)
			} else {
				pc += int(i.Jf)
			}
		case 0x35:
			if a >= i.K {
				pc += int(i.Jt)
			} else {
				pc += int(i.Jf)
			}
		case 0x06:
			return i.K
		default:
			t.Fatal("unknown BPF opcode")
		}
	}
	t.Fatal("unterminated filter")
	return 0
}
func TestClientFilterDeniesOwnerGroupAndAlternateSignalPaths(t *testing.T) {
	for _, nr := range []uint32{unix.SYS_KILL, unix.SYS_TKILL, unix.SYS_RT_SIGQUEUEINFO, unix.SYS_RT_TGSIGQUEUEINFO, unix.SYS_PIDFD_SEND_SIGNAL, unix.SYS_PTRACE, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PIDFD_GETFD, unix.SYS_UNSHARE, unix.SYS_SETNS, unix.SYS_CLONE3, unix.SYS_IO_URING_SETUP} {
		for _, pid := range []uint64{0, 1233, ^uint64(0)} {
			if r := evalEvidenceFilter(t, nr, unix.AUDIT_ARCH_X86_64, [6]uint64{pid, 9}); r != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
				t.Fatal("owner/group escape syscall allowed", nr, pid, r)
			}
		}
	}
	if r := evalEvidenceFilter(t, unix.SYS_TGKILL, unix.AUDIT_ARCH_X86_64, [6]uint64{1233, 1, uint64(unix.SIGURG)}); r == unix.SECCOMP_RET_ALLOW {
		t.Fatal("owner tgkill permitted")
	}
	if r := evalEvidenceFilter(t, unix.SYS_TGKILL, unix.AUDIT_ARCH_X86_64, [6]uint64{1234, 1234, 9}); r == unix.SECCOMP_RET_ALLOW {
		t.Fatal("nonruntime self signal allowed")
	}
	if r := evalEvidenceFilter(t, unix.SYS_TGKILL, unix.AUDIT_ARCH_X86_64, [6]uint64{1234, 1234, uint64(unix.SIGURG)}); r != unix.SECCOMP_RET_ALLOW {
		t.Fatal("runtime self signal refused")
	}
}
func TestClientFilterArchAndCloneBoundaries(t *testing.T) {
	for _, arch := range []uint32{0, unix.AUDIT_ARCH_I386} {
		if evalEvidenceFilter(t, unix.SYS_READ, arch, [6]uint64{}) != unix.SECCOMP_RET_KILL_PROCESS {
			t.Fatal("wrong arch accepted")
		}
	}
	if evalEvidenceFilter(t, 0x40000000, unix.AUDIT_ARCH_X86_64, [6]uint64{}) != unix.SECCOMP_RET_KILL_PROCESS {
		t.Fatal("x32 accepted")
	}
	for _, flags := range []uint64{0, 0xd0f00 | unix.CLONE_NEWUSER, unix.CLONE_NEWNET, 0xd0f00 | 1<<32} {
		if evalEvidenceFilter(t, unix.SYS_CLONE, unix.AUDIT_ARCH_X86_64, [6]uint64{flags}) == unix.SECCOMP_RET_ALLOW {
			t.Fatal("unreviewed clone flags permitted")
		}
	}
	for _, nr := range []uint32{unix.SYS_READ, unix.SYS_WRITE, unix.SYS_FUTEX} {
		if evalEvidenceFilter(t, nr, unix.AUDIT_ARCH_X86_64, [6]uint64{}) != unix.SECCOMP_RET_ALLOW {
			t.Fatal("basic runtime syscall refused")
		}
	}
}
func TestExactSecurebitsGuard(t *testing.T) {
	if clientSecurebits != 47 {
		t.Fatal("securebits no longer exact NOROOT/no-fixup/keepcaps-lock")
	}
}
func TestClientFilterOnlyExactRuntimeClone(t *testing.T) {
	if evalEvidenceFilter(t, unix.SYS_CLONE, unix.AUDIT_ARCH_X86_64, [6]uint64{0xd0f00}) != unix.SECCOMP_RET_ALLOW {
		t.Fatal("exact Go runtime thread mask refused")
	}
}
func TestSecurebitsValueActuallyApplied(t *testing.T) {
	s := sourceFunction(t, "client_exec_live_linux.go", "execUnprivilegedClientSource")
	if !strings.Contains(s, "PR_SET_SECUREBITS, clientSecurebits") {
		t.Fatal("exact tested securebits not applied")
	}
}
func TestTgkillExactTargetEqualitySameSignalDecisive(t *testing.T) {
	args := [6]uint64{1234, 1234, uint64(unix.SIGURG)}
	if evalEvidenceFilter(t, unix.SYS_TGKILL, unix.AUDIT_ARCH_X86_64, args) != unix.SECCOMP_RET_ALLOW {
		t.Fatal("self runtime signal failed")
	}
	for _, target := range []uint64{0, 1233, 1235, ^uint64(0), uint64(1234) | 1<<32} {
		args[0] = target
		if got := evalEvidenceFilter(t, unix.SYS_TGKILL, unix.AUDIT_ARCH_X86_64, args); got != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
			t.Fatal("tgkill target equality missing", target, got)
		}
	}
}
func TestRemainingDenySyscallsIndividually(t *testing.T) {
	for _, nr := range []uint32{unix.SYS_UNSHARE, unix.SYS_PROCESS_VM_READV, unix.SYS_PIDFD_GETFD} {
		if evalEvidenceFilter(t, nr, unix.AUDIT_ARCH_X86_64, [6]uint64{}) != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
			t.Fatal("deny syscall missing", nr)
		}
	}
}
