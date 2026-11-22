//go:build mnet_enrollment_development

package daemon

import (
	"strings"
	"testing"
)

// Pure string parser only. Dummy key-shaped data is not a credential and does
// not create a daemon, peer, bridge, TUN, notifier, factory or WireGuard device.
func TestPrivateEndpointReadbackShapeMemory(t *testing.T) {
	private := strings.Repeat("1", 64)
	peer := strings.Repeat("2", 64)
	ip := "100.64.0.2"
	raw := "private_key=" + private + "\npublic_key=" + peer + "\nallowed_ip=" + ip + "/32\n"
	cases := []struct {
		name, raw string
		want      bool
	}{
		{"one-peer-one-ip", raw, true}, {"zero-mark-omitted", raw + "tx_bytes=0\n", true}, {"explicit-zero-mark", raw + "fwmark=0\n", true},
		{"wrong-private", strings.Replace(raw, private, strings.Repeat("3", 64), 1), false},
		{"duplicate-private", raw + "private_key=" + private + "\n", false},
		{"second-peer", raw + "public_key=" + peer + "\n", false},
		{"second-allowed-ip", raw + "allowed_ip=" + ip + "/32\n", false},
		{"subnet", strings.Replace(raw, "/32", "/24", 1), false},
		{"default-route", strings.Replace(raw, ip+"/32", "0.0.0.0/0", 1), false},
		{"missing-private", strings.TrimPrefix(raw, "private_key="+private+"\n"), false},
		{"nonzero-mark", raw + "fwmark=1\n", false},
		{"ip-before-peer", "private_key=" + private + "\nallowed_ip=" + ip + "/32\npublic_key=" + peer + "\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := privateEndpointConfigMatches(c.raw, private, peer, ip); got != c.want {
				t.Fatal("config shape decision")
			}
		})
	}
}
