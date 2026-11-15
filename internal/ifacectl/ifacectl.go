// Package ifacectl gives the TUN device its address and route with the
// operating system's own tools.
package ifacectl

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Configure sets addr (for example 100.64.0.5) on the interface and routes
// the whole 100.64.0.0/10 mesh range to it.
func Configure(ifname, addr string, mtu int) error {
	if addr == "" {
		return fmt.Errorf("no address assigned yet")
	}
	var cmds [][]string
	switch runtime.GOOS {
	case "linux":
		cmds = [][]string{
			{"ip", "link", "set", "dev", ifname, "mtu", fmt.Sprint(mtu)},
			{"ip", "addr", "replace", addr + "/10", "dev", ifname},
			{"ip", "link", "set", "dev", ifname, "up"},
			{"ip", "route", "replace", "100.64.0.0/10", "dev", ifname},
		}
	case "darwin":
		cmds = [][]string{
			{"ifconfig", ifname, "mtu", fmt.Sprint(mtu)},
			{"ifconfig", ifname, "inet", addr, addr, "netmask", "255.192.0.0", "up"},
			{"route", "-n", "add", "-net", "100.64.0.0/10", "-interface", ifname},
		}
	case "windows":
		cmds = [][]string{
			{"netsh", "interface", "ip", "set", "address", "name=" + ifname, "static", addr, "255.192.0.0"},
		}
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %v: %s", c, err, out)
		}
	}
	return nil
}
