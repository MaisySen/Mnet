//go:build linux && mnet_enrollment_development

package osownership

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// Characterize the compiler sources used for THIS fixed client, including the
// assembly transformation, rather than treating Go's base constant as syscall.
// Pin entire files so source edits require explicit renewed review.
func TestPinnedRuntimeCloneAssemblyOracle(t *testing.T) {
	if !evidenceRuntimeSupported(runtime.GOOS, runtime.GOARCH, runtime.Version()) {
		t.Fatal("runtime requires renewed characterization")
	}
	pins := map[string]string{
		"os_linux.go":       "f211c96802c315ffa23343fc3a672f38713418f46658681da1f4d8187d9462a1",
		"sys_linux_amd64.s": "98ec6f0afedcc07d64f94c299c201b1ca5e7d52f7ed284bf6782c5ca7b9760f2",
		"proc.go":           "b0ea864c1dd251cd9146644863ae9126ca57761dff05862dc80ab0799925fec6",
	}
	for name, want := range pins {
		b, e := os.ReadFile(filepath.Join(runtime.GOROOT(), "src", "runtime", name))
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			t.Fatal("runtime source changed", name)
		}
	}
	asm, e := os.ReadFile(filepath.Join(runtime.GOROOT(), "src", "runtime", "sys_linux_amd64.s"))
	if e != nil {
		t.Fatal(e)
	}
	text := string(asm)
	start := strings.Index(text, "TEXT runtime·clone(SB)")
	if start < 0 {
		t.Fatal("clone assembly missing")
	}
	end := strings.Index(text[start:], "nog1:")
	if end < 0 {
		t.Fatal("clone assembly missing")
	}
	prefix := text[start : start+end]
	for _, step := range []string{"MOVL\tflags+0(FP), DI", "CMPQ\tR13, $0", "CMPQ\tR9, $0", "LEAQ\tm_tls(R13), R8", "ORQ \t$0x00080000, DI"} {
		if !strings.Contains(prefix, step) {
			t.Fatal("assembly transformation missing", step)
		}
	}
	// Values from the pinned runtime's six symbolic thread constants, followed by
	// the separately pinned amd64 assembly TLS addition. No flags union in filter.
	base := uint64(unix.CLONE_VM | unix.CLONE_FS | unix.CLONE_FILES | unix.CLONE_SIGHAND | unix.CLONE_SYSVSEM | unix.CLONE_THREAD)
	actual := base | unix.CLONE_SETTLS
	if actual == base {
		t.Fatal("TLS addition absent")
	}
	if got := evalEvidenceFilter(t, unix.SYS_CLONE, unix.AUDIT_ARCH_X86_64, [6]uint64{actual, 0x1000, 0, 0, 0x2008}); got != unix.SECCOMP_RET_ALLOW {
		t.Fatalf("actual emitted shape refused: %x", got)
	}
	if got := evalEvidenceFilter(t, unix.SYS_CLONE, unix.AUDIT_ARCH_X86_64, [6]uint64{base}); got != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
		t.Fatal("unneeded nil-mp/gp shape allowed")
	}
}

func TestExactTLSCloneAllFlagBitsDecisive(t *testing.T) {
	mask := uint64(unix.CLONE_VM | unix.CLONE_FS | unix.CLONE_FILES | unix.CLONE_SIGHAND | unix.CLONE_SYSVSEM | unix.CLONE_THREAD | unix.CLONE_SETTLS)
	for bit := uint(0); bit < 64; bit++ {
		flags := mask ^ (uint64(1) << bit)
		if got := evalEvidenceFilter(t, unix.SYS_CLONE, unix.AUDIT_ARCH_X86_64, [6]uint64{flags, 0x1000, 0, 0, 0x2008}); got != unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
			t.Fatalf("changed flag bit%d allowed", bit)
		}
	}
}
func TestRuntimeVersionPinDecisive(t *testing.T) {
	if !strings.Contains(sourceFunction(t, "client_filter_linux.go", "evidenceClientFilter"), "!evidenceRuntimeSupported(runtime.GOOS, runtime.GOARCH, runtime.Version())") {
		t.Fatal("runtime pin not applied")
	}
	if !evidenceRuntimeSupported("linux", "amd64", "go1.26.11") {
		t.Fatal("pin rejected")
	}
	for _, v := range [][3]string{{"linux", "amd64", "go1.26.6"}, {"linux", "amd64", "go1.26.12"}, {"linux", "amd64", "go1.26.11-custom"}, {"linux", "arm64", "go1.26.11"}, {"darwin", "amd64", "go1.26.11"}} {
		if evidenceRuntimeSupported(v[0], v[1], v[2]) {
			t.Fatal("unreviewed runtime allowed", v)
		}
	}
}
