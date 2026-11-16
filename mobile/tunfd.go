//go:build android || linux

package mobile

import (
	"os"
	"sync"

	"golang.zx2c4.com/wireguard/tun"

	"fintex.au/mnet/internal/daemon"
)

// fdTun is a minimal tun.Device over an already established VPN tunnel
// file descriptor. App processes on Android get no netlink socket and no
// tun ioctls, so wireguard's CreateTUNFromFile fails with EPERM there;
// a VpnService fd needs only plain packet reads and writes.
type fdTun struct {
	f      *os.File
	mtu    int
	events chan tun.Event
	once   sync.Once
}

func newFDTun(fd int, mtu int) *fdTun {
	t := &fdTun{
		f:      os.NewFile(uintptr(fd), "mnet-tun"),
		mtu:    mtu,
		events: make(chan tun.Event, 1),
	}
	t.events <- tun.EventUp
	return t
}

func (t *fdTun) File() *os.File { return t.f }

func (t *fdTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	n, err := t.f.Read(bufs[0][offset:])
	if err != nil {
		return 0, err
	}
	sizes[0] = n
	return 1, nil
}

func (t *fdTun) Write(bufs [][]byte, offset int) (int, error) {
	for i, b := range bufs {
		if _, err := t.f.Write(b[offset:]); err != nil {
			return i, err
		}
	}
	return len(bufs), nil
}

func (t *fdTun) MTU() (int, error) { return t.mtu, nil }

func (t *fdTun) Name() (string, error) { return "mnet0", nil }

func (t *fdTun) Events() <-chan tun.Event { return t.events }

func (t *fdTun) Close() error {
	var err error
	t.once.Do(func() {
		close(t.events)
		err = t.f.Close()
	})
	return err
}

func (t *fdTun) BatchSize() int { return 1 }

// Start brings the mesh up on an already established tunnel file
// descriptor, such as the ParcelFileDescriptor from Android's
// VpnService.Builder.establish(). The fd is consumed: the daemon closes
// it on Stop. server, network and authKey behave as in Prepare.
func (n *Node) Start(tunFD int, server, network, name, authKey string) error {
	t := newFDTun(tunFD, daemon.MTU)
	if err := n.d.SetTUN(t); err != nil {
		t.Close()
		return err
	}
	if err := n.d.Up(server, network, name, authKey); err != nil {
		t.Close()
		return err
	}
	return nil
}
