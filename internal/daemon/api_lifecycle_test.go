//go:build !windows

package daemon

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestLocalAPIBindReadinessAndClose(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan error, 1)
	done := make(chan error, 1)
	go func() { done <- d.ServeLocalAPIContext(ctx, ready) }()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("no readiness")
	}
	network, addr := SocketPath(dir)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	resp, err := client.Get("http://local/localapi/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("API did not stop")
	}
	if conn, err := net.DialTimeout(network, addr, time.Second); err == nil {
		conn.Close()
		t.Fatal("listener survived cancellation")
	}
}
