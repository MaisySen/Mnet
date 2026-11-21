//go:build linux && mnet_enrollment_development

package osownership

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func sourceFunction(t *testing.T, file, name string) string {
	t.Helper()
	b, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	fs := token.NewFileSet()
	f, e := parser.ParseFile(fs, file, b, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return string(b[fs.Position(fn.Pos()).Offset:fs.Position(fn.End()).Offset])
		}
	}
	t.Fatal("missing function", name)
	return ""
}
func TestSourcePidfdBeforeAnyWaitAndPinThroughChildLifetime(t *testing.T) {
	s := sourceFunction(t, "resident_linux.go", "spawnResidentPinnedSource")
	if strings.Index(s, "unix.PidfdOpen(") < 0 || strings.Index(s, "unix.PidfdOpen(") > strings.Index(s, "cmd.Wait()") {
		t.Fatal("pidfd opened after reaper")
	}
	s = sourceFunction(t, "resident_linux.go", "Spawn")
	for _, term := range []string{"runtime.LockOSThread()", "defer runtime.UnlockOSThread()", "<-child.wait"} {
		if !strings.Contains(s, term) {
			t.Fatal("starter thread lifetime pin missing", term)
		}
	}
}
func TestSourceInheritedVerdictCLOEXECNoParentNamespaceFD(t *testing.T) {
	s := sourceFunction(t, "resident_linux.go", "residentBootstrapSource")
	if strings.Index(s, "unix.CloseOnExec(3)") < 0 || strings.Index(s, "unix.CloseOnExec(3)") > strings.Index(s, "io.ReadFull(") {
		t.Fatal("verdict fd CLOEXEC too late")
	}
	if strings.Count(s, "os.Getppid()") < 2 {
		t.Fatal("parent-alive checks absent")
	}
	s = sourceFunction(t, "resident_linux.go", "residentCommand")
	if !strings.Contains(s, "[]*os.File{verdict}") {
		t.Fatal("parent netns FD inherited")
	}
	if !strings.Contains(s, "MNO_PARENT_DEV=") || !strings.Contains(s, "MNO_PARENT_INO=") {
		t.Fatal("numeric identity absent")
	}
}
func TestSourceClientCapsNNPFDAllowlistBeforeFixedExec(t *testing.T) {
	s := sourceFunction(t, "client_sandbox_linux.go", "execUnprivilegedClientSource")
	last := -1
	for _, term := range []string{"runtime.LockOSThread()", "PR_SET_NO_NEW_PRIVS", "PR_CAP_AMBIENT_CLEAR_ALL", "PR_CAPBSET_DROP", "PR_SET_SECUREBITS", "unix.Capset(", "unix.CloseRange(", "syscall.Exec("} {
		n := strings.Index(s, term)
		if n <= last {
			t.Fatal("client sandbox source missing/out of order", term)
		}
		last = n
	}
	s = sourceFunction(t, "resident_linux.go", "residentBootstrapSource")
	if !strings.Contains(s, "PR_SET_DUMPABLE, 0") || strings.Index(s, "PR_SET_DUMPABLE, 0") < strings.Index(s, "io.ReadFull(") {
		t.Fatal("client can inspect owner fd custody")
	}
}
