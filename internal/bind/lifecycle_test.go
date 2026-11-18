package bind

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
)

func lifecycleKey() string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x4e}, 32)) }
func receiveResult(fn conn.ReceiveFunc) (int, error) {
	return fn([][]byte{make([]byte, 2048)}, make([]int, 1), make([]conn.Endpoint, 1))
}

func TestOpenRejectsAlreadyOpenWithoutReplacingSocket(t *testing.T) {
	b := New()
	defer b.Close()
	_, port, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	original := b.udp
	b.mu.Unlock()
	defer original.Close()
	_, _, err = b.Open(0)
	if !errors.Is(err, conn.ErrBindAlreadyOpen) {
		t.Fatalf("duplicate Open replaced live socket instead of rejecting: %v", err)
	}
	if b.Port() != port {
		t.Fatal("rejected Open changed bound port")
	}
}

func TestReopenDiscardsPreviousRelayQueue(t *testing.T) {
	b := New()
	defer b.Close()
	if _, _, err := b.Open(0); err != nil {
		t.Fatal(err)
	}
	b.Inject(lifecycleKey(), []byte("old-generation"))
	b.Close()
	fns, _, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	b.Inject(lifecycleKey(), []byte("new-generation"))
	data, _ := recvOne(t, fns[1])
	if string(data) != "new-generation" {
		t.Fatalf("reopened receiver consumed %q", data)
	}
}

func TestClosedReceiveFunctionsCannotConsumeReopenedTraffic(t *testing.T) {
	for _, kind := range []int{0, 1} {
		t.Run(itoa(kind), func(t *testing.T) {
			b := New()
			defer b.Close()
			old, _, err := b.Open(0)
			if err != nil {
				t.Fatal(err)
			}
			b.Close()
			fresh, port, err := b.Open(0)
			if err != nil {
				t.Fatal(err)
			}
			if kind == 0 {
				u, e := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(port)})
				if e != nil {
					t.Fatal(e)
				}
				defer u.Close()
				u.Write([]byte("fresh"))
			} else {
				b.Inject(lifecycleKey(), []byte("fresh"))
			}
			done := make(chan error, 1)
			go func() {
				n, e := receiveResult(old[kind])
				if n != 0 && e == nil {
					done <- errors.New("old receiver consumed new-generation data")
				} else {
					done <- e
				}
			}()
			select {
			case e := <-done:
				if !errors.Is(e, net.ErrClosed) {
					t.Fatalf("closed receiver: %v", e)
				}
			case <-time.After(time.Second):
				t.Fatal("old receiver remained live after Close/Open")
			}
			data, _ := recvOne(t, fresh[kind])
			if string(data) != "fresh" {
				t.Fatal("new receiver lost packet")
			}
		})
	}
}

func TestInjectWhileClosedDoesNotSeedNextOpen(t *testing.T) {
	b := New()
	defer b.Close()
	if _, _, err := b.Open(0); err != nil {
		t.Fatal(err)
	}
	b.Close()
	for i := 0; i < 256; i++ {
		b.Inject(lifecycleKey(), []byte("while-closed"))
	}
	fns, _, err := b.Open(0)
	if err != nil {
		t.Fatal(err)
	}
	b.Inject(lifecycleKey(), []byte("fresh"))
	data, _ := recvOne(t, fns[1])
	if string(data) != "fresh" {
		t.Fatalf("post-close injection survived into next Open: %q", data)
	}
}
