//go:build android || linux

package mobile

import (
	"fmt"
	"os"

	"golang.zx2c4.com/wireguard/tun"

	"fintex.au/mnet/internal/daemon"
)

// Start brings the mesh up on an already established tunnel file
// descriptor, such as the ParcelFileDescriptor from Android's
// VpnService.Builder.establish(). The fd is consumed: the daemon closes
// it on Stop. server, network and authKey behave as in Prepare.
func (n *Node) Start(tunFD int, server, network, name, authKey string) error {
	t, err := tun.CreateTUNFromFile(os.NewFile(uintptr(tunFD), "mnet-tun"), daemon.MTU)
	if err != nil {
		return fmt.Errorf("tun from fd: %w", err)
	}
	if err := n.d.SetTUN(t); err != nil {
		t.Close()
		return err
	}
	return n.d.Up(server, network, name, authKey)
}
