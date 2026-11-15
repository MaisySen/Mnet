package relay

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReplacementWhileForwarding(t *testing.T) {
	h := NewHub()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	var handlers sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		handlers.Add(1)
		defer handlers.Done()
		h.Attach("node", key, ws, nil)
	}))
	defer srv.Close()
	done := make(chan struct{})
	var forwarding sync.WaitGroup
	forwarding.Add(1)
	go func() {
		defer forwarding.Done()
		for {
			select {
			case <-done:
				return
			default:
				h.Forward(key, key, []byte("packet"))
			}
		}
	}()
	defer func() { close(done); forwarding.Wait() }()
	var prev *websocket.Conn
	defer func() {
		if prev != nil {
			prev.Close()
		}
	}()
	for i := 0; i < 30; i++ {
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		mt, payload, err := c.ReadMessage()
		if err != nil {
			c.Close()
			t.Fatal(err)
		}
		if mt != websocket.BinaryMessage || len(payload) != KeyLen+6 {
			c.Close()
			t.Fatalf("bad packet: %d %d", mt, len(payload))
		}
		// Leave the previous connection open until replaced, to exercise
		// replacement concurrent with forwarding, without closing send queues.
		if prev != nil {
			prev.Close()
		}
		prev = c
		if i == 29 {
			h.Detach("node")
			c.Close()
		}
	}
	handlers.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.conns) != 0 || len(h.ByNodeID) != 0 {
		t.Fatalf("relay state leaked: %d %d", len(h.conns), len(h.ByNodeID))
	}
}

func TestScopedForwardDoesNotCrossNetworks(t *testing.T) {
	h := NewHub()
	src := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	dst := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	a := &conn{send: make(chan []byte, 1), done: make(chan struct{})}
	b := &conn{send: make(chan []byte, 1), done: make(chan struct{})}
	h.conns["network-a\x00"+dst] = a
	h.conns["network-b\x00"+dst] = b
	if !h.ForwardScoped("network-a", src, dst, []byte("scoped")) {
		t.Fatal("same-network route denied")
	}
	select {
	case <-a.send:
	default:
		t.Fatal("packet not delivered in source scope")
	}
	select {
	case <-b.send:
		t.Fatal("identical key in different network received packet")
	default:
	}
	if h.ForwardScoped("network-missing", src, dst, []byte("cross-network")) {
		t.Fatal("packet escaped source scope")
	}
	if h.ForwardScoped("network-a", src, dst, make([]byte, MaxPacket+1)) {
		t.Fatal("oversized packet admitted")
	}
}
