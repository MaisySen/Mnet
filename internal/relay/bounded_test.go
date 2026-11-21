package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
