//go:build android || linux

package mobile

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"

	"golang.zx2c4.com/wireguard/tun"

	"fintex.au/mnet/internal/daemon"
)

// fdTun is a minimal tun.Device over an already established VPN tunnel
// file descriptor. App processes on Android get no netlink socket and no
// tun ioctls, so wireguard's CreateTUNFromFile fails with EPERM there;
// a VpnService fd needs only plain packet reads and writes.
type fdTun struct {
	f           *os.File
	mtu         int
	events      chan tun.Event
	once        sync.Once
	reads       atomic.Uint64
	writes      atomic.Uint64
	readIPv4    atomic.Bool
	readTCP     atomic.Bool
	readSYN     atomic.Bool
	readFailed  atomic.Bool
	writeFailed atomic.Bool
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
		t.readFailed.Store(true)
		return 0, err
	}
	t.observePacket(bufs[0][offset : offset+n])
	t.reads.Add(1)
	sizes[0] = n
	return 1, nil
}

func (t *fdTun) Write(bufs [][]byte, offset int) (int, error) {
	for i, b := range bufs {
		if _, err := t.f.Write(b[offset:]); err != nil {
			t.writeFailed.Store(true)
			return i, err
		}
		t.writes.Add(1)
	}
	return len(bufs), nil
}

func (t *fdTun) packetDiagnostics() string {
	b, _ := json.Marshal(map[string]bool{"read_ipv4": t.readIPv4.Load(), "read_tcp": t.readTCP.Load(), "read_syn": t.readSYN.Load(), "read_packet": t.reads.Load() > 0, "wrote_packet": t.writes.Load() > 0, "read_failed": t.readFailed.Load(), "write_failed": t.writeFailed.Load()})
	return string(b)
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
	n.diagMu.Lock()
	n.packetDiag = t
	n.diagMu.Unlock()
	if err := n.d.SetTUN(t); err != nil {
		t.Close()
		return err
	}
	if err := n.d.Up(server, network, name, authKey); err != nil {
		n.d.Down()
		t.Close()
		return err
	}
	return nil
}

// Packet classes only. Never retains or exports addresses or payloads.
func (t *fdTun) observePacket(packet []byte) {
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return
	}
	h := int(packet[0]&15) * 4
	if h < 20 || len(packet) < h {
		return
	}
	t.readIPv4.Store(true)
	if packet[9] != 6 || len(packet) < h+20 {
		return
	}
	t.readTCP.Store(true)
	flags := packet[h+13]
	if flags&2 != 0 && flags&16 == 0 {
		t.readSYN.Store(true)
	}
}
