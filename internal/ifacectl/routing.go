package ifacectl

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// DefaultRoute returns the current underlay default gateway and device.
func DefaultRoute() (gw, dev string, err error) {
	switch runtime.GOOS {
	case "linux":
		out, err := exec.Command("ip", "route", "show", "default").CombinedOutput()
		if err != nil {
			return "", "", fmt.Errorf("ip route show default: %v: %s", err, out)
		}
		f := strings.Fields(string(out))
		for i := 0; i+1 < len(f); i++ {
			switch f[i] {
			case "via":
				gw = f[i+1]
			case "dev":
				dev = f[i+1]
			}
		}
		if dev == "" {
			return "", "", fmt.Errorf("no default route found in %q", out)
		}
		return gw, dev, nil
	case "darwin":
		out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
		if err != nil {
			return "", "", fmt.Errorf("route get default: %v: %s", err, out)
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "gateway:") {
				gw = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
			}
			if strings.HasPrefix(line, "interface:") {
				dev = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
			}
		}
		if dev == "" {
			return "", "", fmt.Errorf("no default route found in %q", out)
		}
		return gw, dev, nil
	default:
		return "", "", fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}
