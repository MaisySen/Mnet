package control

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/version"
	"github.com/gorilla/websocket"
	"net/http/httptest"
)

func serveHTTPPolicy(t *testing.T, s *http.Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ln) }()
	t.Cleanup(func() {
		s.Close()
		select {
		case err := <-done:
			if err != http.ErrServerClosed {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("server did not stop")
		}
	})
	return ln.Addr().String()
}

func TestHTTPIngressPolicy(t *testing.T) {
	s := NewHTTPServer("", http.NotFoundHandler())
	if s.ReadHeaderTimeout <= 0 || s.ReadTimeout <= 0 || s.IdleTimeout <= 0 || s.MaxHeaderBytes <= 0 || s.MaxHeaderBytes > 64<<10 {
		t.Fatalf("missing ingress bounds: %+v", s)
	}
	if s.WriteTimeout <= 50*time.Second {
		t.Fatal("write deadline would truncate map long poll")
	}
}

func TestSlowHTTPHeadersExpire(t *testing.T) {
	var called atomic.Bool
	s := NewHTTPServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Store(true) }))
	s.ReadHeaderTimeout = 40 * time.Millisecond
	addr := serveHTTPPolicy(t, s)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = io.WriteString(c, "GET /healthz HTTP/1.1\r\nHost: example.test\r\nX-Incomplete:"); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(time.Second))
	_, err = bufio.NewReader(c).ReadByte()
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("slow header connection remained open")
	}
	if called.Load() {
		t.Fatal("incomplete headers reached handler")
	}
}

func TestSlowHTTPBodyExpires(t *testing.T) {
	readFailed := make(chan error, 1)
	s := NewHTTPServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readFailed <- err
		w.WriteHeader(400)
	}))
	s.ReadTimeout = 50 * time.Millisecond
	addr := serveHTTPPolicy(t, s)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = io.WriteString(c, "POST /api/join HTTP/1.1\r\nHost: example.test\r\nContent-Length: 100\r\n\r\n{"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readFailed:
		if err == nil {
			t.Fatal("incomplete body read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("slow body did not expire")
	}
}

func TestHTTPHeaderSizeBound(t *testing.T) {
	s := NewHTTPServer("", http.NotFoundHandler())
	s.MaxHeaderBytes = 1024
	addr := serveHTTPPolicy(t, s)
	req, _ := http.NewRequest("GET", "http://"+addr+"/healthz", nil)
	req.Header.Set("X-Large", strings.Repeat("x", 12<<10))
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Fatalf("oversize header status %d", resp.StatusCode)
	}
}

func TestHTTPTimeoutsDoNotTruncateWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{}
	s := NewHTTPServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(time.Second))
		mt, b, err := c.ReadMessage()
		if err == nil {
			c.WriteMessage(mt, b)
		}
	}))
	s.ReadTimeout = 40 * time.Millisecond
	s.WriteTimeout = 40 * time.Millisecond
	addr := serveHTTPPolicy(t, s)
	c, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/api/relay/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	time.Sleep(100 * time.Millisecond)
	c.SetReadDeadline(time.Now().Add(time.Second))
	if err = c.WriteMessage(websocket.BinaryMessage, []byte("relay-after-http-deadline")); err != nil {
		t.Fatal(err)
	}
	_, b, err := c.ReadMessage()
	if err != nil || string(b) != "relay-after-http-deadline" {
		t.Fatalf("upgraded connection inherited HTTP deadline: %q %v", b, err)
	}
}

func TestHealthReportsBuildRevision(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Server{}).handleHealthz(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || rec.Body.String() != "ok\n" || rec.Header().Get("X-Mnet-Revision") != version.Revision {
		t.Fatal("health revision metadata missing")
	}
}
