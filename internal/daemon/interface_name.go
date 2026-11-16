package daemon

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"golang.zx2c4.com/wireguard/tun"
)

// DefaultInterfaceName requests an OS-assigned utun on macOS. Other
// platforms retain their established interface name.
func DefaultInterfaceName() string {
	if runtime.GOOS == "darwin" {
		return "utun"
	}
	return "mnet0"
}

func validDarwinKernelName(name string, automatic bool) bool {
	if automatic && name == "utun" {
		return true
	}
	index := strings.TrimPrefix(name, "utun")
	n, err := strconv.ParseUint(index, 10, 16)
	return strings.HasPrefix(name, "utun") && err == nil && strconv.FormatUint(n, 10) == index
}

func validateManagedInterface(name, platform string) error {
	if platform == "darwin" && !validDarwinKernelName(name, true) {
		return fmt.Errorf("macOS requires utun for automatic assignment or a canonical utunN interface name")
	}
	return nil
}

func kernelInterfaceName(t tun.Device, requested, platform string) (string, error) {
	if platform != "darwin" {
		return requested, nil
	}
	name, err := t.Name()
	if err != nil {
		return "", fmt.Errorf("read actual kernel interface name: %w", err)
	}
	if !validDarwinKernelName(name, false) {
		return "", fmt.Errorf("invalid actual macOS kernel interface name")
	}
	return name, nil
}

func (d *Daemon) actualInterfaceName() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.interfaceName
}
