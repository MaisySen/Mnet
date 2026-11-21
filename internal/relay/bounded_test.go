package relay

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBoundedAttachmentFiniteFramesAndLifetime(t *testing.T) {
	for _, mode := range []string{"valid-frame-budget", "malformed-frame-budget", "cancel", "timeout", "invalid-no-deadline"} {
		t.Run(mode, func(t *testing.T) {
			hub := NewHub()
			key := EncodeKey(make([]byte, 32))
			var calls atomic.Int32
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "timeout" {
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
			}
			if mode == "invalid-no-deadline" {
				ctx = context.Background()
			}
			completed := make(chan error, 1)
			attached := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					completed <- err
					return
				}
				close(attached)
				completed <- hub.AttachScopedBounded(ctx, 2, "bounded-node", "bounded-network", key, ws, func(src, dst string, payload []byte) { calls.Add(1) })
			}))
			defer server.Close()
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ws.Close()
			<-attached
			switch mode {
			case "valid-frame-budget", "malformed-frame-budget":
				packet := append(make([]byte, 32), 1)
				if mode == "malformed-frame-budget" {
					packet = []byte{1}
				}
				for i := 0; i < 2; i++ {
					if err := ws.WriteMessage(websocket.BinaryMessage, packet); err != nil {
						t.Fatal(err)
					}
				}
			case "cancel":
				cancel()
			}
			ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			if _, _, err := ws.ReadMessage(); err == nil {
				t.Fatal("finite attachment stayed open")
			}
			select {
			case err := <-completed:
				if (mode == "invalid-no-deadline") != (err != nil) {
					t.Fatal("attachment result", err)
				}
			case <-time.After(time.Second):
				t.Fatal("attachment/socket cleanup incomplete")
			}
			if mode == "valid-frame-budget" && calls.Load() != 2 {
				t.Fatal("valid budget callback count", calls.Load())
			}
			if mode == "malformed-frame-budget" && calls.Load() != 0 {
				t.Fatal("malformed frame reached callback")
			}
			if len(hub.OnlineNodes()) != 0 {
				t.Fatal("bounded attachment retained online authority")
			}
		})
	}
}

func TestBoundedAttachmentInvalidLimitsNeverAttach(t *testing.T) {
	for _, frames := range []int{0, 4097} {
		hub := NewHub()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done := make(chan error, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				done <- err
				return
			}
			done <- hub.AttachScopedBounded(ctx, frames, "node", "network", EncodeKey(make([]byte, 32)), ws, func(string, string, []byte) { t.Error("invalid config forwarded") })
		}))
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := <-done; err == nil {
			t.Fatal("invalid finite config attached")
		}
		if len(hub.OnlineNodes()) != 0 {
			t.Fatal("invalid finite attachment registered")
		}
		ws.Close()
		server.Close()
		cancel()
	}
}

type queuedTestConn struct {
	net.Conn
	armed            atomic.Bool
	entered, release chan struct{}
}

func (c *queuedTestConn) Write(p []byte) (int, error) {
	if c.armed.CompareAndSwap(true, false) {
		close(c.entered)
		<-c.release
	}
	return c.Conn.Write(p)
}

type queuedTestWriter struct {
	http.ResponseWriter
	conn chan *queuedTestConn
}

func (w queuedTestWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, b, err := w.ResponseWriter.(http.Hijacker).Hijack()
	if err != nil {
		return nil, nil, err
	}
	g := &queuedTestConn{Conn: c, entered: make(chan struct{}), release: make(chan struct{})}
	w.conn <- g
	return g, b, nil
}

func TestBoundedQueuedOnlyRevocationBeforeDestinationDrain(t *testing.T) {
	hub := NewHub()
	key := EncodeKey(make([]byte, 32))
	rawKey := make([]byte, 32)
	var allowed atomic.Bool
	allowed.Store(true)
	conns := make(chan *queuedTestConn, 1)
	enqueued := make(chan struct{}, 2)
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(queuedTestWriter{w, conns}, r, nil)
		if err != nil {
			return
		}
		defer close(done)
		hub.AttachScopedBounded(ctx, 8, "node", "network", key, ws, func(src, dst string, payload []byte) {
			// Admission is intentionally independent and always succeeds. Only the
			// queued destination-write callback can observe the later revocation.
			if hub.ForwardScopedChecked("network", src, dst, payload, func() bool { return allowed.Load() }) {
				enqueued <- struct{}{}
			}
		})
	}))
	defer server.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	gate := <-conns
	gate.armed.Store(true)
	release := sync.OnceFunc(func() { close(gate.release) })
	defer release()
	send := func(body string) {
		if err := ws.WriteMessage(websocket.BinaryMessage, append(append([]byte{}, rawKey...), []byte(body)...)); err != nil {
			t.Fatal(err)
		}
	}
	send("first-already-authorized")
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("first destination write never blocked")
	}
	<-enqueued
	send("second-queued-before-revoke")
	select {
	case <-enqueued:
	case <-time.After(time.Second):
		t.Fatal("second frame not queued before revocation")
	}
	allowed.Store(false)
	release()
	ws.SetReadDeadline(time.Now().Add(time.Second))
	_, first, err := ws.ReadMessage()
	if err != nil || string(first[32:]) != "first-already-authorized" {
		t.Fatal("first authorized write did not drain", err)
	}
	ws.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, second, err := ws.ReadMessage(); err == nil {
		t.Fatalf("queued-only revocation bypass delivered %q", second[32:])
	}
	cancel()
	<-done
}
