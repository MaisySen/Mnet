package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gorilla/websocket"
	"golang.zx2c4.com/wireguard/tun"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/relay"
	"fintex.au/mnet/internal/session"
)

// Opt-in stress over the actual userspace mesh, without an X11 dependency.
// This is diagnostic coverage, not an end-user platform validation.
func TestMeshFileStress(t *testing.T) {
	if os.Getenv("MNET_MESH_STRESS") != "1" {
		t.Skip("set MNET_MESH_STRESS=1 for userspace mesh stress")
	}
	dir := t.TempDir()
	store, err := control.Open(filepath.Join(dir, "control.json"))
	if err != nil {
		t.Fatal(err)
	}
	network, err := store.CreateNetwork("stress")
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAuthKey(network.ID, "stress", true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewUnstartedServer(nil)
	server, _, err := control.NewServer(store, relay.NewHub(), "http://"+httpServer.Listener.Addr().String(), "testpassword")
	if err != nil {
		t.Fatal(err)
	}
	httpServer.Config.Handler = server.Handler()
	httpServer.Start()
	defer httpServer.Close()
	baseURL := httpServer.URL
	lossMode := os.Getenv("MNET_MESH_LOSS")
	if lossMode != "" {
		target, _ := url.Parse(httpServer.URL)
		proxy := httputil.NewSingleHostReverseProxy(target)
		var largePackets atomic.Int64
		var dropUntil atomic.Int64
		lossProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/relay/ws" {
				proxy.ServeHTTP(w, r)
				return
			}
			upstream, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+r.URL.RequestURI(), nil)
			if e != nil {
				http.Error(w, "relay unavailable", 502)
				return
			}
			downstream, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if e != nil {
				upstream.Close()
				return
			}
			defer downstream.Close()
			defer upstream.Close()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer downstream.Close()
				for {
					mt, b, e := upstream.ReadMessage()
					if e != nil {
						return
					}
					if downstream.WriteMessage(mt, b) != nil {
						return
					}
				}
			}()
			for {
				mt, b, e := downstream.ReadMessage()
				if e != nil {
					break
				}
				// One controlled encrypted data-datagram loss, never payload inspection.
				if mt == websocket.BinaryMessage && len(b) > 1000 && binary.LittleEndian.Uint32(b[32:36]) == 4 && func() bool {
					n := largePackets.Add(1)
					if n == 5 {
						duration := 20 * time.Millisecond
						if value, e := strconv.Atoi(os.Getenv("MNET_LOSS_MS")); e == nil && value >= 1 && value <= 1000 {
							duration = time.Duration(value) * time.Millisecond
						}
						dropUntil.Store(time.Now().Add(duration).UnixNano())
						return true
					}
					return lossMode == "burst" && time.Now().UnixNano() < dropUntil.Load()
				}() {
					t.Log("injected large WireGuard datagram loss")
					continue
				}
				if upstream.WriteMessage(mt, b) != nil {
					break
				}
			}
			upstream.Close()
			wg.Wait()
		}))
		defer lossProxy.Close()
		baseURL = lossProxy.URL
	}

	host, err := newProofNodeWithTUN(dir, "host", baseURL, "stress", "stress-host", key.Key, false, traceTUN(t, "host"))
	if err != nil {
		t.Fatal(err)
	}
	defer host.d.Down()
	viewer, err := newProofNodeWithTUN(dir, "viewer", baseURL, "stress", "stress-viewer", key.Key, false, traceTUN(t, "viewer"))
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.d.Down()
	ln, err := host.net.ListenTCPAddrPort(netip.AddrPortFrom(host.ip, proofPort))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hostMux := make(chan *session.Mux, 1)
	hostErr := make(chan error, 1)
	received := filepath.Join(dir, "received")
	go func() {
		m, e := session.Listen(ln, "stress-host", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		if e != nil {
			hostErr <- e
			return
		}
		m.Keepalive(time.Second, 60*time.Second)
		m.OnStream(func(s session.Stream) {
			if s.ID >= session.StreamFile {
				session.FileReceiver{Mux: m, Dir: received}.Handle(s)
			}
		})
		hostMux <- m
	}()
	deadline := time.Now().Add(30 * time.Second)
	var client *session.Mux
	for {
		conn, e := viewer.net.DialTCPAddrPort(netip.AddrPortFrom(host.ip, proofPort))
		if e == nil {
			client, e = session.Dial(conn, "stress-viewer")
			if e == nil {
				break
			}
			conn.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("mesh dial: %v", e)
		}
		time.Sleep(time.Second)
	}
	defer client.Close()
	client.Keepalive(time.Second, 60*time.Second)
	select {
	case m := <-hostMux:
		defer m.Close()
	case err := <-hostErr:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("host setup")
	}
	for i := 0; i < 100; i++ {
		size := 256*1024 + i*97
		data := bytes.Repeat([]byte{byte(i)}, size)
		name := fmt.Sprintf("payload-%03d.bin", i)
		src := filepath.Join(dir, name)
		if err := os.WriteFile(src, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := session.SendFile(client, src); err != nil {
			t.Fatalf("transfer %d: %v; host=%+v viewer=%+v", i, err, host.d.PeerStatuses(), viewer.d.PeerStatuses())
		}
		got, err := os.ReadFile(filepath.Join(received, name))
		if err != nil || !bytes.Equal(data, got) {
			t.Fatalf("transfer %d mismatched: %v", i, err)
		}
	}
}

// Packet metadata only: retain no IP addresses or application bytes.
type diagnosticTUN struct {
	tun.Device
	mu     sync.Mutex
	events []string
	name   string
	next   uint64
}

func traceTUN(t *testing.T, name string) func(tun.Device) tun.Device {
	return func(d tun.Device) tun.Device {
		if os.Getenv("MNET_PACKET_TRACE") != "1" {
			return d
		}
		dt := &diagnosticTUN{Device: d, name: name, events: make([]string, 512)}
		t.Cleanup(func() {
			if t.Failed() {
				dt.mu.Lock()
				defer dt.mu.Unlock()
				start := uint64(0)
				if dt.next > 512 {
					start = dt.next - 512
				}
				for i := start; i < dt.next; i++ {
					t.Log(dt.events[i%512])
				}
			}
		})
		return dt
	}
}
func (d *diagnosticTUN) record(direction string, p []byte) {
	if len(p) < 40 || p[0]>>4 != 4 || p[9] != 6 {
		return
	}
	h := int(p[0]&15) * 4
	if len(p) < h+20 {
		return
	}
	tcp := p[h:]
	hdr := int(tcp[12]>>4) * 4
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events[d.next%512] = fmt.Sprintf("%s %s at=%s seq=%d ack=%d flags=%02x bytes=%d", d.name, direction, time.Now().Format(time.RFC3339Nano), binary.BigEndian.Uint32(tcp[4:8]), binary.BigEndian.Uint32(tcp[8:12]), tcp[13], len(tcp)-hdr)
	d.next++
}
func (d *diagnosticTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	n, e := d.Device.Read(bufs, sizes, offset)
	for i := 0; i < n; i++ {
		d.record("out", bufs[i][offset:offset+sizes[i]])
	}
	return n, e
}
func (d *diagnosticTUN) Write(bufs [][]byte, offset int) (int, error) {
	for _, b := range bufs {
		d.record("in", b[offset:])
	}
	return d.Device.Write(bufs, offset)
}
