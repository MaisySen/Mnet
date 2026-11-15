package ifacectl

import (
	"fmt"
	"net"
	"net/url"
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

// HostRoute pins one host (the tunnel endpoint, the control server) to the
// underlay gateway so the tunnel survives its own default-route takeover.
func HostRoute(hostport, gw string) error {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("host route: %q is not an IP", host)
	}
	return addHostRoute(ip.String(), gw)
}

// HostRouteURL resolves a server URL to IPs and pins host routes for them.
func HostRouteURL(rawurl, gw string) error {
	u, err := url.Parse(rawurl)
	if err != nil {
		return err
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return addHostRoute(host, gw)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			return addHostRoute(ip4.String(), gw)
		}
	}
	return fmt.Errorf("no A record for %s", host)
}

func addHostRoute(ip, gw string) error {
	switch runtime.GOOS {
	case "linux":
		out, err := exec.Command("ip", "route", "replace", ip+"/32", "via", gw).CombinedOutput()
		if err != nil {
			return fmt.Errorf("host route %s via %s: %v: %s", ip, gw, err, out)
		}
		return nil
	case "darwin":
		out, err := exec.Command("route", "-n", "change", "-host", ip, gw).CombinedOutput()
		if err != nil {
			return fmt.Errorf("host route %s via %s: %v: %s", ip, gw, err, out)
		}
		return nil
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}

// DefaultRouteVia points the default route into the tunnel device. Split
// 0.0.0.0/1 + 128.0.0.0/1 beats the underlay default without deleting it,
// so Down restores connectivity cleanly.
func DefaultRouteVia(ifname string) error {
	switch runtime.GOOS {
	case "linux":
		for _, cidr := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
			out, err := exec.Command("ip", "route", "replace", cidr, "dev", ifname).CombinedOutput()
			if err != nil {
				return fmt.Errorf("route %s dev %s: %v: %s", cidr, ifname, err, out)
			}
		}
		return nil
	case "darwin":
		return fmt.Errorf("darwin exit-node routing is not wired yet; set the utun default route manually")
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}
