package main

import (
	"context"
	"errors"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/localapi"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
)

type testDesktopView struct {
	mu     sync.Mutex
	end    bool
	closed bool
	frames int
	state  string
	fail   error
	polled chan struct{}
	once   sync.Once
}

func (v *testDesktopView) Frame(session.ScreenFrame) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return errors.New("render after close")
	}
	v.frames++
	return v.fail
}
func (v *testDesktopView) State(s string) error { v.state = s; return v.fail }
func (v *testDesktopView) Poll() (bool, error) {
	v.once.Do(func() {
		if v.polled != nil {
			close(v.polled)
		}
	})
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.end, nil
}
func (v *testDesktopView) Close() error { v.closed = true; return nil }

func TestDesktopViewWindowCloseCancelsAndJoinsPendingDial(t *testing.T) {
	v := &testDesktopView{end: true}
	canceled := make(chan struct{})
	err := runDesktopView(context.Background(), v, func(ctx context.Context) (*session.Mux, error) { <-ctx.Done(); close(canceled); return nil, ctx.Err() })
	if err != nil || !v.closed {
		t.Fatalf("viewer cleanup: %v closed=%v", err, v.closed)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("pending dial not joined")
	}
}
func TestDesktopViewStartupFailureStillJoinsPendingDial(t *testing.T) {
	expected := errors.New("display failed")
	v := &testDesktopView{fail: expected}
	canceled := make(chan struct{})
	err := runDesktopView(context.Background(), v, func(ctx context.Context) (*session.Mux, error) { <-ctx.Done(); close(canceled); return nil, ctx.Err() })
	if !errors.Is(err, expected) || !v.closed {
		t.Fatalf("cleanup: %v", err)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("dial not joined")
	}
}
func TestDesktopViewDialFailureDoesNotReconnect(t *testing.T) {
	v := &testDesktopView{}
	calls := 0
	expected := errors.New("host refused")
	err := runDesktopView(context.Background(), v, func(context.Context) (*session.Mux, error) { calls++; return nil, expected })
	if !errors.Is(err, expected) || calls != 1 || !v.closed {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
func TestDesktopViewCancellationDoesNotWaitForPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v := &testDesktopView{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runDesktopView(ctx, v, func(ctx context.Context) (*session.Mux, error) { <-ctx.Done(); return nil, ctx.Err() })
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("viewer did not join cancellation")
	}
}

func TestDesktopViewCancellationReachesLocalAuthorization(t *testing.T) {
	dir := t.TempDir()
	token, err := localapi.EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	network, addr := daemon.SocketPath(dir)
	if network != "unix" {
		t.Skip("requires disposable Unix socket")
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	entered := make(chan struct{})
	exited := make(chan struct{})
	server := &http.Server{Handler: localapi.RequireToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(exited) }))}
	go server.Serve(ln)
	defer server.Close()
	previous := stateDir
	stateDir = dir
	defer func() { stateDir = previous }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := prepareDesktopView(ctx, "host"); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("local authorization request not made")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled evidence request accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("local authorization did not cancel")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("local request retained after viewer cancellation")
	}
}
