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
// outbound packet. Cancel must be idempotent, return promptly and wake every
// blocked ReadPacket with an error without waiting for another OS packet.
// It must also reject late callback data and future reads/writes.
type PacketFlow interface {
	ReadPacket() ([]byte, error)
	WritePacket(pkt []byte) error
	Cancel()
}

// StartFlow brings the mesh up over a PacketFlow instead of a file
// descriptor. server, network and authKey behave as in Prepare.
func (n *Node) StartFlow(flow PacketFlow, server, network, name, authKey string) error {
	if flow == nil {
		return errors.New("nil packet flow")
	}
	t := newFlowTUN(flow)
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

// flowTUN adapts a PacketFlow to tun.Device, one packet per call.
type flowTUN struct {
	flow   PacketFlow
	events chan tun.Event

	closeOnce sync.Once
	closeCh   chan struct{}
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
	if len(bufs) == 0 || len(sizes) == 0 || offset < 0 || offset > len(bufs[0]) {
		return 0, errors.New("invalid packet read buffer")
	}
	select {
	case <-f.closeCh:
		return 0, errors.New("flow closed")
	default:
	}
	pkt, err := f.flow.ReadPacket()
	if err != nil {
		return 0, err
	}
	select {
	case <-f.closeCh:
		return 0, errors.New("flow closed")
	default:
	}
	if len(pkt) > len(bufs[0])-offset {
		return 0, errors.New("packet larger than buffer")
	}
	sizes[0] = copy(bufs[0][offset:], pkt)
	return 1, nil
}

func (f *flowTUN) Write(bufs [][]byte, offset int) (int, error) {
	for i, buf := range bufs {
		select {
		case <-f.closeCh:
			return i, errors.New("flow closed")
		default:
		}
		if offset < 0 || len(buf) < offset {
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
	f.closeOnce.Do(func() {
		close(f.closeCh)
		close(f.events)
		f.flow.Cancel()
	})
	return nil
}

func (f *flowTUN) BatchSize() int { return 1 }
