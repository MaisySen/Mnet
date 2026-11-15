package mobile

import (
	"errors"
	"os"
	"sync"

	"golang.zx2c4.com/wireguard/tun"

	"fintex.au/mnet/internal/daemon"
)

// PacketFlow is how platforms without a tunnel file descriptor move
// packets: NEPacketTunnelFlow on iOS. ReadPacket blocks until a packet
// is available or the flow is cancelled; WritePacket delivers one
// outbound packet.
type PacketFlow interface {
	ReadPacket() ([]byte, error)
	WritePacket(pkt []byte) error
}

// StartFlow brings the mesh up over a PacketFlow instead of a file
// descriptor. server, network and authKey behave as in Prepare.
func (n *Node) StartFlow(flow PacketFlow, server, network, name, authKey string) error {
	if flow == nil {
		return errors.New("nil packet flow")
	}
	if err := n.d.SetTUN(newFlowTUN(flow)); err != nil {
		return err
	}
	return n.d.Up(server, network, name, authKey)
}

// flowTUN adapts a PacketFlow to tun.Device, one packet per call.
type flowTUN struct {
	flow   PacketFlow
	events chan tun.Event

	closeMu sync.Mutex
	closed  bool
	closeCh chan struct{}
}

func newFlowTUN(flow PacketFlow) *flowTUN {
	return &flowTUN{
		flow:    flow,
		events:  make(chan tun.Event, 4),
		closeCh: make(chan struct{}),
	}
}

// File returns nil: a PacketFlow has no file descriptor. The daemon
// never touches it; only platform socket code would.
func (f *flowTUN) File() *os.File { return nil }

func (f *flowTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	select {
	case <-f.closeCh:
		return 0, errors.New("flow closed")
	default:
	}
	pkt, err := f.flow.ReadPacket()
	if err != nil {
		return 0, err
	}
	if len(bufs) == 0 {
		return 0, nil
	}
	if len(pkt)+offset > len(bufs[0]) {
		return 0, errors.New("packet larger than buffer")
	}
	sizes[0] = copy(bufs[0][offset:], pkt)
	return 1, nil
}

func (f *flowTUN) Write(bufs [][]byte, offset int) (int, error) {
	for i, buf := range bufs {
		if len(buf) < offset {
			return i, errors.New("offset beyond packet")
		}
		if err := f.flow.WritePacket(buf[offset:]); err != nil {
			return i, err
		}
	}
	return len(bufs), nil
}

func (f *flowTUN) MTU() (int, error) { return daemon.MTU, nil }

func (f *flowTUN) Name() (string, error) { return "mnet-flow", nil }

func (f *flowTUN) Events() <-chan tun.Event { return f.events }

func (f *flowTUN) Close() error {
	f.closeMu.Lock()
	defer f.closeMu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.closeCh)
		close(f.events)
	}
	return nil
}

func (f *flowTUN) BatchSize() int { return 1 }
