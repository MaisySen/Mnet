package daemon

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClosedNodeCannotRestart(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "unexpected", 500) }))
	defer server.Close()
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	d.Close()
	if _, err = d.Prepare(server.URL, "fixture", "node", ""); err == nil {
		t.Fatal("closed Prepare accepted")
	}
	if err = d.Up(server.URL, "fixture", "node", ""); err == nil {
		t.Fatal("closed Up accepted")
	}
	if err = d.SetTUN(nil); err == nil {
		t.Fatal("closed tunnel injection accepted")
	}
	if calls.Load() != 0 || d.Status().Status != "down" || d.State().PrivateKey != "" {
		t.Fatal("closed node mutated/enrolled")
	}
}

func TestCloseCancelsBlockedPreparePermanently(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	prepared := make(chan error, 1)
	go func() { _, err := d.Prepare(server.URL, "fixture", "node", ""); prepared <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Prepare did not enter request")
	}
	closed := make(chan struct{})
	go func() { d.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel blocked preparation")
	}
	if err := <-prepared; err == nil {
		t.Fatal("canceled Prepare succeeded")
	}
	if _, err := d.Prepare(server.URL, "fixture", "node", ""); err == nil {
		t.Fatal("closed preparation revived")
	}
}

func TestCloseVersusNewStartup(t *testing.T) {
	for i := 0; i < 30; i++ {
		d, err := New(Config{StateDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { _ = d.Up("http://127.0.0.1:1", "fixture", "node", ""); close(done) }()
		d.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("startup escaped permanent Close")
		}
		if d.Status().Status != "down" {
			t.Fatal("closed startup still running")
		}
		if err := d.Up("http://127.0.0.1:1", "fixture", "node", ""); err == nil {
			t.Fatal("closed node restarted")
		}
	}
}
func TestCloseCancelsBlockedUpPermanently(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	prepared := make(chan error, 1)
	go func() { err := d.Up(server.URL, "fixture", "node", ""); prepared <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Up did not enter request")
	}
	closed := make(chan struct{})
	go func() { d.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel blocked Up")
	}
	if err := <-prepared; err == nil {
		t.Fatal("canceled Up succeeded")
	}
	if err := d.Up(server.URL, "fixture", "node", ""); err == nil {
		t.Fatal("closed preparation revived")
	}
}
