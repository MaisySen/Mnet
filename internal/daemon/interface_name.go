package daemon

import (
	"fmt"
	"golang.zx2c4.com/wireguard/tun"
	"runtime"
	"strconv"
	"strings"
)

// DefaultInterfaceName requests an OS-assigned utun on macOS. Other
// platforms retain their established interface name.
func DefaultInterfaceName() string {
	if runtime.GOOS == "darwin" {
		return "utun"
	}
	return "mnet0"
}

func kernelInterfaceName(t tun.Device, requested, platform string) (string, error) {
	if platform != "darwin" {
		return requested, nil
	}
	name, err := t.Name()
	if err != nil {
		return "", fmt.Errorf("read actual kernel interface name: %w", err)
	}
	index := strings.TrimPrefix(name, "utun")
	n, err := strconv.ParseUint(index, 10, 16)
	if !strings.HasPrefix(name, "utun") || err != nil || strconv.FormatUint(n, 10) != index {
		return "", fmt.Errorf("invalid actual macOS kernel interface name")
	}
	return name, nil
}

func (d *Daemon) actualInterfaceName() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.interfaceName
}
