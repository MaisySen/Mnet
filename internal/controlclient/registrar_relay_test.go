package controlclient

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/relay"
	"github.com/gorilla/websocket"
)

func TestRegistrarRelayDedicatedHeaderLifecycle(t *testing.T) {
	for _, mode := range []string{"Close", "timeout", "frame-budget", "denied"} {
		t.Run(mode, func(t *testing.T) {
			var doc contract.NativeCredentialDocument
			var calls atomic.Int32
			bad := make(chan struct{}, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, err := base64.RawURLEncoding.Strict().DecodeString(r.Header.Get("X-Mnet-Relay-Reference-V1"))
				wire, parseErr := contract.DecodeHistoryReceiptWire(raw, doc.Challenge)
				if r.URL.Path != "/api/mnet-node-relay-experimental-v1/ws" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Mnet-Node-Relay-V1 "+doc.Secret || err != nil || parseErr != nil || wire.Receipt != doc.Receipt || strings.Contains(string(raw), doc.Secret) {
					select {
					case bad <- struct{}{}:
					default:
					}
					w.WriteHeader(400)
					return
				}
				if mode == "denied" {
					w.WriteHeader(403)
					return
				}
				ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer ws.Close()
				for {
					typ, msg, err := ws.ReadMessage()
					if err != nil {
						return
					}
					if err := ws.WriteMessage(typ, msg); err != nil {
						return
					}
				}
			}))
			defer server.Close()
			doc = registrarFixture(t, server.URL)
			client := registrarTrusted(t, server, doc, RegistrarLimits{Timeout: 200 * time.Millisecond, MaxConcurrent: 1, MaxRequests: 4})
			received := make(chan []byte, 2)
			maxFrames := 8
			if mode == "frame-budget" {
				maxFrames = 2
			}
			socket, err := client.DialRegistrarRelay(context.Background(), maxFrames, func(src string, p []byte) { received <- append([]byte{}, p...) })
			if mode == "denied" {
				if err != ErrRegistrarDenied || socket != nil {
					t.Fatal("permanent dedicated denial", err)
				}
				if _, err := client.DialRegistrarRelay(context.Background(), 8, func(string, []byte) {}); err != ErrRegistrarDenied || calls.Load() != 1 {
					t.Fatal("denied client retried")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer socket.Close()
			key := relay.EncodeKey(make([]byte, 32))
			if err := socket.Send(key, []byte("opaque-test-frame")); err != nil {
				t.Fatal(err)
			}
			select {
			case p := <-received:
				if string(p) != "opaque-test-frame" {
					t.Fatal("frame changed")
				}
			case <-time.After(time.Second):
				t.Fatal("dedicated relay frame absent")
			}
			switch mode {
			case "Close":
				client.Close()
			case "frame-budget":
				if err := socket.Send(key, []byte("over-budget")); err == nil {
					t.Fatal("per-connection total frame budget reset")
				}
			}
			select {
			case <-socket.Done():
			case <-time.After(time.Second):
				t.Fatal("finite socket retained after lifecycle end")
			}
			if err := socket.Send(key, []byte("late")); err == nil {
				t.Fatal("closed finite socket sent")
			}
			select {
			case <-bad:
				t.Fatal("dedicated auth/reference envelope differs")
			default:
			}
		})
	}
}
