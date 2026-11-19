package controlclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/relay"
	"github.com/gorilla/websocket"
)

// A WebSocket read failure ends the relay, but does not itself close the
// underlying TCP connection. Done must not leave the old socket retained
// while the daemon retries on a new connection.
func TestRelayReadFailureClosesOwnedSocketBeforeDone(t *testing.T) {
	for _, mode := range []string{"oversized-packet", "remote-close"} {
		t.Run(mode, func(t *testing.T) {
			finish := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					finish <- err
					return
				}
				defer ws.Close()
				if mode == "oversized-packet" {
					err = ws.WriteMessage(websocket.BinaryMessage, make([]byte, relay.MaxPacket+relay.KeyLen+17))
				} else {
					err = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "fixture closed"), time.Now().Add(time.Second))
				}
				if err != nil {
					finish <- err
					return
				}
				conn := ws.UnderlyingConn()
				if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					finish <- err
					return
				}
				_, err = io.Copy(io.Discard, conn)
				finish <- err
			}))
			defer server.Close()
			client := &Client{Base: server.URL, NodeID: "fixture-node", Token: "fixture-token"}
			connection, err := client.DialRelay(context.Background(), "", func(string, []byte) { t.Error("invalid packet reached callback") })
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			select {
			case <-connection.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("failed relay never ended")
			}
			select {
			case err := <-finish:
				var timeout net.Error
				if errors.As(err, &timeout) && timeout.Timeout() {
					t.Fatal("Done left relay TCP socket open")
				}
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("peer did not observe relay socket close")
			}
			if err := connection.Send(strings.Repeat("A", 43)+"=", []byte("late")); err == nil {
				t.Fatal("ended relay accepted a send")
			}
		})
	}
}

func TestRelayReaderCleanupKeepsIndependentConnectionUsable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		if r.URL.Query().Get("node_id") == "retired" {
			_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "fixture closed"), time.Now().Add(time.Second))
			_ = ws.UnderlyingConn().SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = io.Copy(io.Discard, ws.UnderlyingConn())
			return
		}
		for {
			kind, payload, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if err := ws.WriteMessage(kind, payload); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	received := make(chan string, 1)
	healthy, err := (&Client{Base: server.URL, NodeID: "healthy"}).DialRelay(context.Background(), "", func(_ string, payload []byte) { received <- string(payload) })
	if err != nil {
		t.Fatal(err)
	}
	defer healthy.Close()
	retired, err := (&Client{Base: server.URL, NodeID: "retired"}).DialRelay(context.Background(), "", func(string, []byte) { t.Error("closed relay received a packet") })
	if err != nil {
		t.Fatal(err)
	}
	defer retired.Close()
	select {
	case <-retired.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("retired reader did not end")
	}
	_ = retired.Close()
	_ = retired.Close()
	if err := healthy.Send(relay.EncodeKey(make([]byte, relay.KeyLen)), []byte("independent relay remains usable")); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-received:
		if payload != "independent relay remains usable" {
			t.Fatal("packet changed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retired reader disrupted independent connection")
	}
	_ = healthy.Close()
	select {
	case <-healthy.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("explicit Close did not end reader")
	}
}
