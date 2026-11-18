package bind

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
)

func TestConcurrentOpenHasExactlyOneOwner(t *testing.T) {
	b := New()
	defer b.Close()
	const workers = 16
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, _, err := b.Open(0); results <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, conn.ErrBindAlreadyOpen) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("concurrent Open produced %d socket owners", success)
	}
}

func TestCloseUnblocksBothGenerationReceivers(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		b := New()
		fns, _, err := b.Open(0)
		if err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{}, 2)
		results := make(chan error, 2)
		for _, fn := range fns {
			go func(fn conn.ReceiveFunc) { entered <- struct{}{}; _, err := receiveResult(fn); results <- err }(fn)
		}
		<-entered
		<-entered
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
		// Reopen immediately, before waiting on old consumers. Neither old call may
		// follow mutable fields into this generation or wait on its cancellation.
		if _, _, err := b.Open(0); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			select {
			case err := <-results:
				if !errors.Is(err, net.ErrClosed) {
					b.Close()
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				b.Close()
				t.Fatal("old receive did not finish after Close")
			}
		}
		b.Close()
	}
}

func TestFailedOpenCanRetryWithoutChangingOwnership(t *testing.T) {
	occupied, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	b := New()
	defer b.Close()
	if _, _, err := b.Open(uint16(occupied.LocalAddr().(*net.UDPAddr).Port)); err == nil {
		t.Fatal("occupied port accepted")
	}
	if b.Port() != 0 {
		t.Fatal("failed Open published a port")
	}
	fns, _, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	for _, fn := range fns {
		if _, err := receiveResult(fn); !errors.Is(err, net.ErrClosed) {
			t.Fatal("retry generation stayed live", err)
		}
	}
}
