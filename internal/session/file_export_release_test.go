package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type leasedFileExport struct {
	checkedFileExport
	released atomic.Int32
}

func (x *leasedFileExport) Release() { x.released.Add(1) }

func TestFileExportSelectedCapabilityReleasedWithoutOpen(t *testing.T) {
	for _, mode := range []string{"reset-during-stage", "budget", "stage-setup", "authorization", "digest", "success"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			adapter := &leasedFileExport{}
			root := filepath.Join(t.TempDir(), "stage")
			if mode == "stage-setup" {
				if err := os.WriteFile(root, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "budget" {
				gate.exportBudget.active = maxActiveFileExports
			}
			var authorized atomic.Bool
			authorized.Store(true)
			done := make(chan struct{})
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, RequireApproval: true, Approval: gate, Authorized: authorized.Load}.Handle(s)
			})
			digest := sha256.Sum256([]byte("abc"))
			offer := FileOffer{Name: "safe.bin", Size: 3, SHA256: hex.EncodeToString(digest[:])}
			if mode == "digest" {
				offer.SHA256 = string(bytes.Repeat([]byte("0"), 64))
			}
			stream, err := client.Open(PriBulk, offer)
			if err != nil {
				t.Fatal(err)
			}
			request := approvalRequest(t, gate)
			if mode == "authorization" {
				authorized.Store(false)
			}
			selection := newFileExportSelection(adapter)
			if err := gate.decideExport(request, root, selection); err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Recv(time.Second); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "reset-during-stage":
				select {
				case <-request.Done:
				default:
					t.Fatal("successful choice did not retire prompt")
				}
				if adapter.released.Load() != 0 {
					t.Fatal("prompt Done prematurely released selected output")
				}
				client.tryReset(stream.ID)
			case "digest", "success":
				if err := client.SendBytes(stream.ID, PriBulk, []byte("abc")); err != nil {
					t.Fatal(err)
				}
				if err := client.send(stream.ID, msgFin, PriBulk, nil); err != nil {
					t.Fatal(err)
				}
				if _, err := stream.Recv(time.Second); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("receiver did not retire")
			}
			if got := adapter.released.Load(); got != 1 {
				t.Fatalf("selected capability release count=%d, want1", got)
			}
			if mode != "success" && len(adapter.events) != 0 {
				t.Fatal("unverified or failed selection opened provider", adapter.events)
			}
		})
	}
}

func TestFileExportRejectedSelectionStaysCallerOwned(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
	})
	if _, err := client.Open(PriBulk, FileOffer{Name: "safe.bin"}); err != nil {
		t.Fatal(err)
	}
	request := approvalRequest(t, gate)
	adapter := &leasedFileExport{}
	selection := newFileExportSelection(adapter)
	bad := request
	bad.ID = "not-current"
	if gate.decideExport(bad, t.TempDir(), selection) == nil {
		t.Fatal("foreign request accepted")
	}
	if adapter.released.Load() != 0 {
		t.Fatal("rejected submission consumed caller-owned capability")
	}
	selection.release()
	selection.release()
	if adapter.released.Load() != 1 {
		t.Fatal("caller release was not one-use")
	}
	if gate.decideExport(request, t.TempDir(), selection) == nil {
		t.Fatal("released selection reused")
	}
	if err := gate.Decide(request, false, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("declined receiver retained")
	}
}

func TestFileExportDecisionDataRaceReleasesExactlyOneOwner(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		client, host := pipe(t)
		gate := NewFileApprovalGate(0)
		done := make(chan struct{})
		host.OnStream(func(s Stream) {
			defer close(done)
			FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
		})
		stream, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 1})
		if err != nil {
			t.Fatal(err)
		}
		request := approvalRequest(t, gate)
		adapter := &leasedFileExport{}
		selection := newFileExportSelection(adapter)
		decided := make(chan error, 1)
		root := t.TempDir()
		go func() { decided <- gate.decideExport(request, root, selection) }()
		if err := client.SendBytes(stream.ID, PriBulk, []byte("x")); err != nil {
			t.Fatal(err)
		}
		client.tryReset(stream.ID)
		if err := <-decided; err != nil {
			selection.release()
		} // rejected submission remains local
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("decision/data race retained receiver")
		}
		if adapter.released.Load() != 1 {
			t.Fatalf("race release=%d", adapter.released.Load())
		}
		gate.Close()
		host.Close()
		client.Close()
	}
}
