//go:build !windows

package daemon

import (
	"context"
	"fintex.au/mnet/internal/localapi"
	"net"
	"net/http"
	"strings"
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
	client := &http.Client{Transport: localapi.Transport{StateDir: dir, Base: transport}, Timeout: time.Second}
	resp, err := client.Get("http://mnetd/localapi/status")
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

func TestLocalAPIRejectsUnauthenticatedAndMalformedControl(t *testing.T) {
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
	if err = <-ready; err != nil {
		t.Fatal(err)
	}
	network, addr := SocketPath(dir)
	base := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	defer base.CloseIdleConnections()
	unauth := &http.Client{Transport: base, Timeout: time.Second}
	for _, path := range []string{"/localapi/status", "/localapi/down"} {
		resp, err := unauth.Post("http://mnetd"+path, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("unauthorized %s status %d", path, resp.StatusCode)
		}
	}
	auth := &http.Client{Transport: localapi.Transport{StateDir: dir, Base: base}, Timeout: time.Second}
	resp, err := auth.Post("http://mnetd/localapi/up", "application/json", strings.NewReader("invalid-json"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 || d.Status().Status != "down" {
		t.Fatal("malformed control request started daemon")
	}
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
