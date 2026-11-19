package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nativeOutputFixture struct {
	bytes.Buffer
	mode            string
	entered         chan struct{}
	unblock         chan struct{}
	canceled        atomic.Int32
	opens, finishes int
	once            sync.Once
}

func (n *nativeOutputFixture) wait(mode string) error {
	if n.mode == mode || n.mode == "late-"+mode {
		close(n.entered)
		<-n.unblock
		if n.mode == "late-"+mode {
			return nil
		}
		return errors.New("private provider cancellation detail")
	}
	if n.mode == "fail-"+mode {
		return errors.New("private provider failure detail")
	}
	return nil
}
func (n *nativeOutputFixture) Open() error { n.opens++; return n.wait("open") }
func (n *nativeOutputFixture) Write(p []byte) error {
	if err := n.wait("write"); err != nil {
		return err
	}
	_, err := n.Buffer.Write(p)
	return err
}
func (n *nativeOutputFixture) Finish() error { n.finishes++; return n.wait("finish") }
func (n *nativeOutputFixture) Cancel()       { n.canceled.Add(1); n.once.Do(func() { close(n.unblock) }) }
func fixtureNativeOutput(mode string) *nativeOutputFixture {
	return &nativeOutputFixture{mode: mode, entered: make(chan struct{}), unblock: make(chan struct{})}
}
func TestNativeFileExportRealReceiverCompletionAndFailures(t *testing.T) {
	for _, mode := range []string{"success", "fail-open", "fail-write", "fail-finish"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			native := fixtureNativeOutput(mode)
			adapter := newNativeFileExport(native)
			done := make(chan struct{})
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
			})
			source := filepath.Join(t.TempDir(), "safe.bin")
			if err := os.WriteFile(source, []byte("abc"), 0600); err != nil {
				t.Fatal(err)
			}
			sent := make(chan error, 1)
			go func() { sent <- SendFile(client, source) }()
			request := approvalRequest(t, gate)
			if err := gate.decideExport(request, t.TempDir(), newFileExportSelection(adapter)); err != nil {
				t.Fatal(err)
			}
			if err := <-sent; (err == nil) != (mode == "success") {
				t.Fatal("wrong native completion", err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("native receiver retained")
			}
			if native.canceled.Load() != 1 {
				t.Fatal("native output not released exactly once")
			}
			select {
			case <-adapter.watched:
			default:
				t.Fatal("native watcher not joined")
			}
			if mode == "success" && (native.String() != "abc" || native.opens != 1 || native.finishes != 1) {
				t.Fatal("wrong exact native output")
			}
		})
	}
}
func TestNativeFileExportRealReceiverEndCancelsBlockedCalls(t *testing.T) {
	for _, mode := range []string{"open", "write", "finish", "late-open", "late-write", "late-finish"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			native := fixtureNativeOutput(mode)
			adapter := newNativeFileExport(native)
			done := make(chan struct{})
			notified := false
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, RequireApproval: true, Approval: gate, OnDone: func(string, int64) { notified = true }}.Handle(s)
			})
			source := filepath.Join(t.TempDir(), "safe.bin")
			if err := os.WriteFile(source, []byte("abc"), 0600); err != nil {
				t.Fatal(err)
			}
			sent := make(chan error, 1)
			go func() { sent <- SendFile(client, source) }()
			if err := gate.decideExport(approvalRequest(t, gate), t.TempDir(), newFileExportSelection(adapter)); err != nil {
				t.Fatal(err)
			}
			select {
			case <-native.entered:
			case <-time.After(time.Second):
				t.Fatal("native callback not entered")
			}
			host.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("native cancellation did not release callback")
			}
			select {
			case err := <-sent:
				if err == nil {
					t.Fatal("ended native export succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("sender retained")
			}
			if notified || native.canceled.Load() != 1 {
				t.Fatal("incorrect terminal native ownership")
			}
			select {
			case <-adapter.watched:
			default:
				t.Fatal("native watcher not joined")
			}
		})
	}
}
func TestNativeFileExportReleaseBeforeOpenAndOneUse(t *testing.T) {
	native := fixtureNativeOutput("success")
	adapter := newNativeFileExport(native)
	if _, err := adapter.Write(context.Background(), []byte("x")); err == nil {
		t.Fatal("write before Open accepted")
	}
	adapter.Release()
	adapter.Release()
	if _, err := adapter.Open(context.Background(), FileOffer{}); err == nil {
		t.Fatal("released native output opened")
	}
	if native.opens != 0 || native.canceled.Load() != 1 {
		t.Fatal("unopened native lifetime not released")
	}
	native = fixtureNativeOutput("success")
	adapter = newNativeFileExport(native)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := adapter.Open(ctx, FileOffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Open(ctx, FileOffer{}); err == nil {
		t.Fatal("native output reopened")
	}
	if err := adapter.Finish(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Write(ctx, []byte("x")); err == nil {
		t.Fatal("native write after finish accepted")
	}
	adapter.Release()
	if native.canceled.Load() != 1 {
		t.Fatal("completed native lifetime release missing")
	}
}
