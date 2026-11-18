package mobile

import (
	"bytes"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeFlow is a scripted PacketFlow: inbound packets queued, writes recorded.
type fakeFlow struct {
	in      chan []byte
	writes  [][]byte
	done    chan struct{}
	once    sync.Once
	cancels atomic.Int32
	reading chan struct{}
}

func newFakeFlow() *fakeFlow {
	return &fakeFlow{in: make(chan []byte, 8), done: make(chan struct{}), reading: make(chan struct{}, 8)}
}

func (f *fakeFlow) ReadPacket() ([]byte, error) {
	select {
	case f.reading <- struct{}{}:
	default:
	}
	select {
	case <-f.done:
		return nil, errors.New("flow ended")
	case p := <-f.in:
		return p, nil
	}
}

func (f *fakeFlow) Cancel() { f.cancels.Add(1); f.once.Do(func() { close(f.done) }) }
func (f *fakeFlow) WritePacket(pkt []byte) error {
	select {
	case <-f.done:
		return errors.New("flow ended")
	default:
	}
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
	<-flow.reading // initial round-trip read
	<-flow.reading // now blocked
	ft.Close()
	ft.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked read succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("Close left foreign read blocked")
	}
	if flow.cancels.Load() != 1 {
		t.Fatal("flow cancellation not exactly once")
	}
	if n, err := ft.Write([][]byte{{1}}, 0); err == nil || n != 0 {
		t.Fatal("post-close write accepted")
	}
	if n, err := ft.Read(bufs, sizes, 0); err == nil || n != 0 {
		t.Fatal("post-close read accepted")
	}

}

type latePacketFlow struct {
	*fakeFlow
	release chan struct{}
}

func (f *latePacketFlow) ReadPacket() ([]byte, error) {
	f.reading <- struct{}{}
	<-f.release
	return []byte{1, 2, 3}, nil
}
func TestFlowTUNRefusesLateReadAndMalformedBuffers(t *testing.T) {
	flow := &latePacketFlow{fakeFlow: newFakeFlow(), release: make(chan struct{})}
	ft := newFlowTUN(flow)
	done := make(chan error, 1)
	go func() { _, err := ft.Read([][]byte{make([]byte, 10)}, make([]int, 1), 0); done <- err }()
	<-flow.reading
	ft.Close()
	close(flow.release)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("late read delivered after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("late read stuck")
	}
	for _, offset := range []int{-1, 11} {
		if _, err := ft.Read([][]byte{make([]byte, 10)}, make([]int, 1), offset); err == nil {
			t.Fatal("invalid read offset accepted")
		}
	}
	if _, err := ft.Read(nil, nil, 0); err == nil {
		t.Fatal("missing read buffers accepted")
	}
	other := newFlowTUN(newFakeFlow())
	defer other.Close()
	if _, err := other.Write([][]byte{{1}}, -1); err == nil {
		t.Fatal("negative write offset accepted")
	}
}
func TestFlowTUNConcurrentCloseCallsCancelOnce(t *testing.T) {
	flow := newFakeFlow()
	ft := newFlowTUN(flow)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); ft.Close() }()
	}
	wg.Wait()
	if flow.cancels.Load() != 1 {
		t.Fatal("duplicate foreign cancellation", flow.cancels.Load())
	}
	select {
	case _, ok := <-ft.Events():
		if ok {
			t.Fatal("events not closed")
		}
	default:
		t.Fatal("events still open")
	}
}
