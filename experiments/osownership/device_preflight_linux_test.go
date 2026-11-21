//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight

package osownership

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/sys/unix"
	"os"
	"strings"
	"testing"
)

type fakeDevice struct {
	m, fm                                      DeviceMetadata
	metaErr, openErr, statErr, closeErr, nsErr error
	fd, flags, opens, closes, inventories      int
	leak                                       bool
	nsCalls, nsFailAt, statusFlags             int
}

func goodDevice() *fakeDevice {
	m := DeviceMetadata{Device: 7, Inode: 99, Rdev: unix.Mkdev(10, 200), Mode: unix.S_IFCHR | 0600}
	return &fakeDevice{m: m, fm: m, fd: 8, flags: unix.FD_CLOEXEC, statusFlags: unix.O_NONBLOCK}
}
func (f *fakeDevice) CheckNamespace() error {
	f.nsCalls++
	if f.nsFailAt == f.nsCalls {
		return unix.EXDEV
	}
	return f.nsErr
}
func (f *fakeDevice) Metadata() (DeviceMetadata, error) { return f.m, f.metaErr }
func (f *fakeDevice) Open() (int, error)                { f.opens++; return f.fd, f.openErr }
func (f *fakeDevice) Fstat(int) (DeviceMetadata, error) { return f.fm, f.statErr }
func (f *fakeDevice) FDFlags(int) (int, error)          { return f.flags, nil }
func (f *fakeDevice) StatusFlags(int) (int, error)      { return f.statusFlags, nil }
func (f *fakeDevice) Close(int) error                   { f.closes++; return f.closeErr }
func (f *fakeDevice) Inventory() ([]int, error) {
	f.inventories++
	if f.leak && f.inventories > 1 {
		return []int{0, 1, 2, 8}, nil
	}
	return []int{0, 1, 2}, nil
}
func TestDevicePreflightPureSuccess(t *testing.T) {
	f := goodDevice()
	r, e := captureDevicePreflight(f, Namespace{Device: 7, Inode: 42, Isolated: true})
	if e != nil || r.Outcome != "opened-closed" || !r.Closed || !r.CloseAttempted || !r.CLOEXEC || !r.FDInventoryUnchanged || !r.NamespaceRechecked || f.opens != 1 || f.closes != 1 {
		t.Fatal(r, e, f)
	}
}
func TestDevicePreflightMissingDeniedValid(t *testing.T) {
	for _, n := range []error{unix.ENOENT, unix.EPERM, unix.EACCES} {
		for _, atOpen := range []bool{false, true} {
			f := goodDevice()
			if atOpen {
				f.openErr = n
			} else {
				f.metaErr = n
			}
			r, e := captureDevicePreflight(f, Namespace{})
			if e != nil || r.Errno == 0 || r.Outcome == "error" || !r.FDInventoryUnchanged || !r.NamespaceRechecked || f.closes != 0 || atOpen != (f.opens == 1) {
				t.Fatal(r, e)
			}
		}
	}
}
func TestDevicePreflightRefusesObject(t *testing.T) {
	for _, kind := range []uint32{unix.S_IFLNK, unix.S_IFREG, unix.S_IFDIR} {
		f := goodDevice()
		f.m.Mode = kind
		r, e := captureDevicePreflight(f, Namespace{})
		if e != nil || r.Outcome != "refused-object" || f.opens != 0 {
			t.Fatal(r, e)
		}
	}
	f := goodDevice()
	f.m.Rdev = unix.Mkdev(1, 3)
	r, e := captureDevicePreflight(f, Namespace{})
	if e != nil || r.OpenAttempted {
		t.Fatal(r, e)
	}
}
func TestDevicePreflightFailureClosesOnce(t *testing.T) {
	for _, kind := range []string{"identity", "stat", "cloexec", "close", "leak"} {
		f := goodDevice()
		switch kind {
		case "identity":
			f.fm.Inode++
		case "stat":
			f.statErr = unix.EIO
		case "cloexec":
			f.flags = 0
		case "close":
			f.closeErr = unix.EIO
		case "leak":
			f.leak = true
		}
		r, e := captureDevicePreflight(f, Namespace{})
		if e == nil || f.closes != 1 || !r.CloseAttempted {
			t.Fatal(kind, r, e)
		}
		if kind == "close" && r.Closed {
			t.Fatal("false close success")
		}
	}
}
func TestDevicePreflightUnknownFailureNotFeasibility(t *testing.T) {
	f := goodDevice()
	f.metaErr = errors.New("opaque")
	if _, e := captureDevicePreflight(f, Namespace{}); e == nil {
		t.Fatal("unknown accepted")
	}
	f = goodDevice()
	f.nsErr = unix.EPERM
	r, e := captureDevicePreflight(f, Namespace{})
	if e == nil || r.OpenAttempted {
		t.Fatal(r, e)
	}
}
func TestDevicePreflightExactOpenFlags(t *testing.T) {
	if preflightDevicePath != "/dev/net/tun" || preflightOpenFlags != unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW {
		t.Fatal("open scope changed")
	}
}

func TestDevicePreflightNamespaceAtEveryBoundary(t *testing.T) {
	for at := 1; at <= 4; at++ {
		f := goodDevice()
		f.nsFailAt = at
		r, e := captureDevicePreflight(f, Namespace{})
		if e == nil {
			t.Fatal(at, r)
		}
		if at <= 2 && f.opens != 0 {
			t.Fatal("open after namespace failure")
		}
		if at >= 3 && f.closes != 1 {
			t.Fatal("failure leak")
		}
	}
}
func TestDevicePreflightNonblockRequired(t *testing.T) {
	f := goodDevice()
	f.statusFlags = 0
	r, e := captureDevicePreflight(f, Namespace{})
	if e == nil || r.Nonblock || f.closes != 1 {
		t.Fatal(r, e)
	}
}

// Strict direct-call surface: unexpected effects fail even if an inserted helper
// is not invoked by fake tests. No ioctl or dynamic launcher may be added.
func TestDevicePreflightNoMutationSurface(t *testing.T) {
	b, e := os.ReadFile("device_preflight_linux.go")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(string(b), "//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight") {
		t.Fatal("tag bypass")
	}
	f, e := parser.ParseFile(token.NewFileSet(), "device_preflight_linux.go", b, 0)
	if e != nil {
		t.Fatal(e)
	}
	allowed := map[string]bool{"unix.Lstat": true, "unix.Open": true, "unix.Fstat": true, "unix.FcntlInt": true, "unix.Close": true, "unix.Major": true, "unix.Minor": true, "errors.As": true, "errors.Join": true, "reflect.DeepEqual": true, "ops.CheckNamespace": true, "ops.Inventory": true, "ops.Metadata": true, "ops.Open": true, "ops.Fstat": true, "ops.FDFlags": true, "ops.StatusFlags": true, "ops.Close": true, "ns.checkCurrent": true, "ctx.Err": true, "o.ns.checkCurrent": true}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		var name string
		if id, ok := sel.X.(*ast.Ident); ok {
			name = id.Name + "." + sel.Sel.Name
		} else if inner, ok := sel.X.(*ast.SelectorExpr); ok {
			if id, ok := inner.X.(*ast.Ident); ok {
				name = id.Name + "." + inner.Sel.Name + "." + sel.Sel.Name
			}
		}
		if !allowed[name] {
			t.Error("unexpected call", name)
		}
		return true
	})
	if !strings.Contains(string(b), "unix.Lstat(preflightDevicePath") || !strings.Contains(string(b), "unix.Open(preflightDevicePath, preflightOpenFlags, 0)") {
		t.Fatal("path/open policy changed")
	}
	if kernelExecutionReleased {
		t.Fatal("ordinary executor enabled")
	}
	if _, e := openExclusiveNonpersistentTUN(nil, "blocked"); !errors.Is(e, ErrExecutorUnreleased) {
		t.Fatal("TUN guard bypass", e)
	}
}

func TestDevicePreflightRealNamespaceAndFlagWiring(t *testing.T) {
	if !strings.Contains(sourceFunction(t, "device_preflight_linux.go", "CheckNamespace"), "o.ns.checkCurrent()") {
		t.Fatal("held namespace bypass")
	}
	for _, tc := range [][2]string{{"Metadata", "unix.Lstat(preflightDevicePath"}, {"Fstat", "unix.Fstat(fd"}, {"FDFlags", "unix.F_GETFD"}, {"StatusFlags", "unix.F_GETFL"}, {"Close", "unix.Close(fd)"}} {
		if !strings.Contains(sourceFunction(t, "device_preflight_linux.go", tc[0]), tc[1]) {
			t.Fatal("real operation wiring changed", tc[0])
		}
	}
}
