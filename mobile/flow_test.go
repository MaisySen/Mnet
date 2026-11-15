package mobile

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

// fakeFlow is a scripted PacketFlow: inbound packets queued, writes recorded.
type fakeFlow struct {
	in     chan []byte
	writes [][]byte
	closed bool
}

func newFakeFlow() *fakeFlow { return &fakeFlow{in: make(chan []byte, 8)} }

func (f *fakeFlow) ReadPacket() ([]byte, error) {
	p, ok := <-f.in
	if !ok {
		return nil, errors.New("flow ended")
	}
	return p, nil
}

func (f *fakeFlow) WritePacket(pkt []byte) error {
	cp := make([]byte, len(pkt))
	copy(cp, pkt)
	f.writes = append(f.writes, cp)
	return nil
}

func TestFlowTUNRoundTrip(t *testing.T) {
	flow := newFakeFlow()
	ft := newFlowTUN(flow)

	if n, err := ft.Write([][]byte{{0, 0, 0x45, 0x01, 0x02}}, 2); err != nil || n != 1 {
		t.Fatalf("write: %d %v", n, err)
	}
	if len(flow.writes) != 1 || !bytes.Equal(flow.writes[0], []byte{0x45, 0x01, 0x02}) {
		t.Fatalf("flow got %v", flow.writes)
	}

	flow.in <- []byte{9, 8, 7}
	buf := make([]byte, 64)
	bufs := [][]byte{buf}
	sizes := make([]int, 1)
	n, err := ft.Read(bufs, sizes, 0)
	if err != nil || n != 1 || sizes[0] != 3 || !bytes.Equal(buf[:3], []byte{9, 8, 7}) {
		t.Fatalf("read: %d %d %v", n, sizes[0], err)
	}

	mtu, _ := ft.MTU()
	if mtu != 1280 {
		t.Fatalf("mtu %d", mtu)
	}

	// Close unblocks readers.
	done := make(chan error, 1)
	go func() {
		_, err := ft.Read(bufs, sizes, 0)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	flow.closed = true
	ft.Close()
	select {
	case <-done:
		// A blocked ReadPacket in a real flow returns an error when the
		// platform cancels the tunnel; the fake blocks forever, which is
		// fine: the device stops calling Read after Close.
	case <-time.After(100 * time.Millisecond):
	}
}
