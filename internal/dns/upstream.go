package dns

import (
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
)

var (
	upstreamOnce sync.Once
	upstream     string
)

// upstreamServer picks the first nameserver from /etc/resolv.conf,
// defaulting to Cloudflare when there is none (Windows, containers).
func upstreamServer() string {
	upstreamOnce.Do(func() {
		upstream = "1.1.1.1:53"
		b, err := os.ReadFile("/etc/resolv.conf")
		if err != nil {
			return
		}
		upstream = chooseUpstream(string(b))
	})
	return upstream
}

// Never forward a request back into either Mnet resolver address range.
func chooseUpstream(config string) string {
	mesh := netip.MustParsePrefix("100.64.0.0/10")
	for _, line := range strings.Split(config, "\n") {
		f := strings.Fields(line)

		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		ip, err := netip.ParseAddr(f[1])
		if err != nil || ip.Unmap().String() == LocalIPv4 || mesh.Contains(ip.Unmap()) {
			continue
		}
		return net.JoinHostPort(ip.String(), "53")
	}
	return "1.1.1.1:53"
}
