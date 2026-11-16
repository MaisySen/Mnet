package daemon

import (
	"errors"
	"golang.zx2c4.com/wireguard/tun"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type namedFixtureTUN struct {
	tun.Device
	name string
	err  error
}

func (t namedFixtureTUN) Name() (string, error) { return t.name, t.err }

func TestKernelInterfaceName(t *testing.T) {
	for _, requested := range []string{"utun", "utun35"} {
		got, err := kernelInterfaceName(namedFixtureTUN{name: "utun57"}, requested, "darwin")
		if err != nil || got != "utun57" {
			t.Fatal("actual kernel name not propagated", got, err)
		}
	}
	for _, name := range []string{"", "utun", "utun-1", "utun01", "utun65536", "mnet0", "utun1\n"} {
		if _, err := kernelInterfaceName(namedFixtureTUN{name: name}, "utun", "darwin"); err == nil {
			t.Fatal("invalid kernel name accepted")
		}
	}
	if _, err := kernelInterfaceName(namedFixtureTUN{err: errors.New("fixture")}, "utun", "darwin"); err == nil {
		t.Fatal("kernel Name error lost")
	}
	for _, platform := range []string{"linux", "windows"} {
		got, err := kernelInterfaceName(namedFixtureTUN{err: errors.New("must not query")}, "mnet0", platform)
		if err != nil || got != "mnet0" {
			t.Fatal("non-Darwin behavior changed")
		}
	}
	want := "mnet0"
	if runtime.GOOS == "darwin" {
		want = "utun"
	}
	if DefaultInterfaceName() != want {
		t.Fatal("wrong platform default")
	}
}

func TestActualInterfaceStatus(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status().Interface != "" {
		t.Fatal("down daemon reports requested interface as actual")
	}
	d.interfaceName = "utun57"
	if d.actualInterfaceName() != "utun57" || d.Status().Interface != "utun57" {
		t.Fatal("status lost actual interface")
	}
	d.stopDevice()
	if d.Status().Interface != "" {
		t.Fatal("teardown retained interface name")
	}
}

func TestStartDeviceFailureClearsInterfaceStatus(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	d.interfaceName = "utun57"
	d.state.PrivateKey = "invalid"
	if err := d.startDevice(); err == nil {
		t.Fatal("invalid key accepted")
	}
	if d.Status().Interface != "" {
		t.Fatal("failed startup retained stale interface")
	}
}

func TestManagedDarwinInterfaceValidation(t *testing.T) {
	for _, name := range []string{"utun", "utun0", "utun9", "utun65535"} {
		if err := validateManagedInterface(name, "darwin"); err != nil {
			t.Fatal("valid requested name rejected", err)
		}
	}
	for _, name := range []string{"", "mnet0", "utun-1", "utun01", "utun65536", "utun7junk", "utun7\n", "utun+7"} {
		if validateManagedInterface(name, "darwin") == nil {
			t.Fatal("invalid requested name accepted")
		}
		if validateManagedInterface(name, "linux") != nil || validateManagedInterface(name, "windows") != nil {
			t.Fatal("non-Darwin behavior changed")
		}
	}
}

func TestDarwinInvalidManagedInterfaceBeforeState(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native Darwin New boundary")
	}
	for _, name := range []string{"mnet0", "utun01", "utun7junk", "utun65536"} {
		root := filepath.Join(t.TempDir(), "not-created")
		if d, err := New(Config{StateDir: root, IfName: name, ManagedIface: true}); err == nil || d != nil {
			t.Fatal("invalid managed request accepted")
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("invalid request touched state", err)
		}
	}
	// Supplied mobile/proof TUNs do not use managed host names.
	if _, err := New(Config{StateDir: t.TempDir(), IfName: "mobile-provided", ManagedIface: false}); err != nil {
		t.Fatal(err)
	}
}
