package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/localapi"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	if runtime.GOOS == "windows" {
		t.Skip("requires disposable Unix socket")
	}
	longRoot := filepath.Join(t.TempDir(), strings.Repeat("long-default-root-", 8))
	if err := os.Mkdir(longRoot, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", longRoot)
	dir, err := os.MkdirTemp("/tmp", "mnet-view-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("fixture directory must be private", err)
	}
	token, err := localapi.EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	network, addr := daemon.SocketPath(dir)
	if len(addr) >= 100 {
		t.Fatal("fixture socket path exceeds conservative Unix bound")
	}
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

type cancelConnectedDesktopView struct {
	testDesktopView
	cancel context.CancelFunc
	mux    *session.Mux
}

func (v *cancelConnectedDesktopView) Poll() (bool, error) {
	if strings.HasPrefix(v.state, "Connected.") {
		v.cancel()
		v.mux.Close()
	}
	return false, nil
}
func TestDesktopViewOwnerCancellationWinsSimultaneousTransportClose(t *testing.T) {
	for _, ownerCanceled := range []bool{false, true} {
		for range 50 {
			ctx, cancel := context.WithCancel(context.Background())
			a, b := net.Pipe()
			go func() {
				if consumeSessionHello(b) != nil {
					return
				}
				payload, _ := json.Marshal(session.HelloReply{Version: 1, Accepted: true})
				var header [12]byte
				header[4] = 2
				binary.BigEndian.PutUint32(header[8:], uint32(len(payload)))
				b.Write(header[:])
				b.Write(payload)
			}()
			mux, dialErr := session.Dial(a, "fixture")
			if dialErr != nil {
				t.Fatal(dialErr)
			}
			stop := func() {}
			if ownerCanceled {
				stop = cancel
			}
			v := &cancelConnectedDesktopView{cancel: stop, mux: mux}
			err := runDesktopView(ctx, v, func(context.Context) (*session.Mux, error) { return mux, nil })
			b.Close()
			cancel()
			if (err == nil) != ownerCanceled || !v.closed {
				t.Fatal("closure/cancellation classification differs", ownerCanceled, err)
			}
			if err != nil && !strings.Contains(err.Error(), "host or path closed") {
				t.Fatal(err)
			}
		}
	}
}

// A late dial completion can race owner cancellation before any connected UI.
// Cancellation must join it and retain local resource-close failures.
type desktopCloseFailure struct {
	testDesktopView
	closeErr error
}

func (v *desktopCloseFailure) Close() error { v.closed = true; return v.closeErr }
func TestDesktopViewCanceledDialStillJoinsAndPreservesCloseFailure(t *testing.T) {
	sentinel := errors.New("fixture display close failure")
	for _, lateFailure := range []bool{false, true} {
		for _, closeFailure := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			v := &desktopCloseFailure{}
			if closeFailure {
				v.closeErr = sentinel
			}
			joined := make(chan struct{})
			err := runDesktopView(ctx, v, func(context.Context) (*session.Mux, error) {
				defer close(joined)
				cancel()
				if lateFailure {
					return nil, errors.New("late canceled dial failure")
				}
				return nil, nil
			})
			if errors.Is(err, sentinel) != closeFailure || (err != nil) != closeFailure || !v.closed {
				t.Fatal(lateFailure, closeFailure, err)
			}
			select {
			case <-joined:
			default:
				t.Fatal("canceled dial not joined")
			}
		}
	}
}
