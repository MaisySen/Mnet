package dns

import (
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
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && f[0] == "nameserver" && !strings.HasPrefix(f[1], "100.64.") {
				upstream = f[1] + ":53"
				return
			}
		}
	})
	return upstream
}
