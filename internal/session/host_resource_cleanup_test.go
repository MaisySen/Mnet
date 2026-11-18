package session

import (
	"sync/atomic"
	"testing"
)

type ownedHostBackend struct{ closes atomic.Int32 }

func (b *ownedHostBackend) Geometry() (int, int)           { return 16, 16 }
func (b *ownedHostBackend) Capture() ([]byte, bool, error) { return nil, false, nil }
func (b *ownedHostBackend) Inject(InputEvent) error        { return nil }
func (b *ownedHostBackend) Close() error                   { b.closes.Add(1); return nil }
func TestHostStopClosesOwnedInputAndCaptureOnce(t *testing.T) {
	for _, shared := range []bool{false, true} {
		client, host := pipe(t)
		cap := &ownedHostBackend{}
		inj := &ownedHostBackend{}
		if shared {
			inj = cap
		}
		h := ServeHost(host, cap, inj, 8)
		h.Stop()
		h.Stop()
		client.Close()
		host.Close()
		if cap.closes.Load() != 1 || inj.closes.Load() != 1 {
			t.Fatal("owned cleanup count", shared, cap.closes.Load(), inj.closes.Load())
		}
	}
}
