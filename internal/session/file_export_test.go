package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type checkedFileExport struct {
	bytes.Buffer
	events  []string
	fail    string
	offer   FileOffer
	opened  chan struct{}
	release chan struct{}
}

func (x *checkedFileExport) Open(ctx context.Context, offer FileOffer) (fileExportOutput, error) {
	x.events = append(x.events, "open")
	x.offer = offer
	if x.opened != nil {
		close(x.opened)
		select {
		case <-x.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if x.fail == "nil" {
		return nil, nil
	}
	if x.fail == "partial-open" {
		return x, errors.New("private provider detail")
	}
	if x.fail == "open" {
		return nil, errors.New("private provider detail")
	}
	return x, nil
}
func (x *checkedFileExport) Write(ctx context.Context, p []byte) (int, error) {
	x.events = append(x.events, "write")
	if x.fail == "write" {
		return 0, errors.New("private provider detail")
	}
	if x.fail == "short" {
		return len(p) - 1, nil
	}
	return x.Buffer.Write(p)
}
func (x *checkedFileExport) Finish(ctx context.Context) error {
	x.events = append(x.events, "finish")
	if x.fail == "finish" {
		return errors.New("private provider detail")
	}
	return nil
}
func (x *checkedFileExport) Abort() { x.events = append(x.events, "abort") }

func TestFileExportRealReceiverVerifiedBytesAndFailures(t *testing.T) {
	for _, mode := range []string{"success", "open", "nil", "partial-open", "write", "short", "finish", "digest"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			adapter := &checkedFileExport{fail: mode}
			selection := newFileExportSelection(adapter)
			root := filepath.Join(t.TempDir(), "private-stage")
			done := make(chan struct{})
			notified := false
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, RequireApproval: true, Approval: gate, OnDone: func(string, int64) { notified = true }}.Handle(s)
			})
			sum := sha256.Sum256([]byte("abc"))
			digest := hex.EncodeToString(sum[:])
			offer := FileOffer{Name: "safe.bin", Size: 3, SHA256: digest}
			if mode == "digest" {
				offer.SHA256 = string(bytes.Repeat([]byte("0"), 64))
			}
			s, err := client.Open(PriBulk, offer)
			if err != nil {
				t.Fatal(err)
			}
			req := approvalRequest(t, gate)
			if len(adapter.events) != 0 {
				t.Fatal("adapter opened before approval")
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("stage before approval")
			}
			if err := gate.decideExport(req, root, selection); err != nil {
				t.Fatal(err)
			}
			f, err := s.Recv(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var accept FileAccept
			if json.Unmarshal(f.Payload, &accept) != nil || !accept.OK {
				t.Fatal("approved staging refused")
			}
			if err := client.SendBytes(s.ID, PriBulk, []byte("abc")); err != nil {
				t.Fatal(err)
			}
			if err := client.send(s.ID, msgFin, PriBulk, nil); err != nil {
				t.Fatal(err)
			}
			f, err = s.Recv(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var ack FileAck
			if json.Unmarshal(f.Payload, &ack) != nil || ack.OK != (mode == "success") {
				t.Fatal("wrong export completion", ack)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("export did not retire")
			}
			if mode == "digest" {
				if len(adapter.events) != 0 {
					t.Fatal("unverified bytes opened export")
				}
			} else {
				if adapter.offer != offer {
					t.Fatal("export offer changed")
				}
				want := []string{"open"}
				if mode == "partial-open" {
					want = append(want, "abort")
				} else if mode != "open" && mode != "nil" {
					want = append(want, "write")
					if mode != "write" && mode != "short" {
						want = append(want, "finish")
					}
					want = append(want, "abort")
				}
				if !reflect.DeepEqual(want, adapter.events) {
					t.Fatal("wrong export order", adapter.events, want)
				}
			}
			if mode == "success" && adapter.String() != "abc" {
				t.Fatal("verified bytes changed")
			}
			if notified != (mode == "success") {
				t.Fatal("failed export notified success")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("staging leaked", entries, err)
			}
		})
	}
}

func TestFileExportSelectionOneUseAndNoStageBeforeDecline(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	adapter := &checkedFileExport{}
	selection := newFileExportSelection(adapter)
	root := filepath.Join(t.TempDir(), "private-stage")
	finished := make(chan uint32, 2)
	host.OnStream(func(s Stream) {
		defer func() { finished <- s.ID }()
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
	})
	for i := 0; i < 2; i++ {
		if _, err := client.Open(PriBulk, FileOffer{Name: "same.bin", Size: 0}); err != nil {
			t.Fatal(err)
		}
	}
	req1, req2 := approvalRequest(t, gate), approvalRequest(t, gate)
	bad := req1
	bad.Offer.Name = "changed.bin"
	if gate.decideExport(bad, root, selection) == nil {
		t.Fatal("changed request selected export")
	}
	if gate.decideExport(req1, "relative", selection) == nil {
		t.Fatal("relative staging selected")
	}
	// A malformed answer cannot consume the capability or the real pending prompt.
	if err := gate.decideExport(req1, root, selection); err != nil {
		t.Fatal(err)
	}
	if gate.decideExport(req2, root, selection) == nil {
		t.Fatal("selection reused for sibling")
	}
	if err := gate.Decide(req2, false, ""); err != nil {
		t.Fatal(err)
	}
	host.Close()
	for i := 0; i < 2; i++ {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("pending receiver leaked")
		}
	}
	if len(adapter.events) != 0 {
		t.Fatal("unverified/declined bytes opened export")
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) != 0 {
		t.Fatal("staging leaked")
	}
}

type blockingFileExport struct {
	entered chan struct{}
	release chan struct{}
	mode    string
	aborted chan struct{}
}

func (x *blockingFileExport) Open(ctx context.Context, offer FileOffer) (fileExportOutput, error) {
	if x.mode == "open" {
		close(x.entered)
		<-x.release
	}
	return x, nil
}
func (x *blockingFileExport) Write(ctx context.Context, p []byte) (int, error) {
	if x.mode == "write" {
		close(x.entered)
		select {
		case <-x.release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return len(p), nil
}
func (x *blockingFileExport) Finish(ctx context.Context) error {
	if x.mode == "finish" {
		close(x.entered)
		select {
		case <-x.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (x *blockingFileExport) Abort() { close(x.aborted) }

func TestFileExportEndRejectsLateOpenWriteAndFinish(t *testing.T) {
	for _, mode := range []string{"open", "write", "finish"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			adapter := &blockingFileExport{mode: mode, entered: make(chan struct{}), release: make(chan struct{}), aborted: make(chan struct{})}
			root := filepath.Join(t.TempDir(), "private-stage")
			done := make(chan struct{})
			notified := false
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, RequireApproval: true, Approval: gate, OnDone: func(string, int64) { notified = true }}.Handle(s)
			})
			s, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 3})
			if err != nil {
				t.Fatal(err)
			}
			req := approvalRequest(t, gate)
			if err := gate.decideExport(req, root, newFileExportSelection(adapter)); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Recv(time.Second); err != nil {
				t.Fatal(err)
			}
			if err := client.SendBytes(s.ID, PriBulk, []byte("abc")); err != nil {
				t.Fatal(err)
			}
			if err := client.send(s.ID, msgFin, PriBulk, nil); err != nil {
				t.Fatal(err)
			}
			select {
			case <-adapter.entered:
			case <-time.After(time.Second):
				t.Fatal("adapter never entered")
			}
			host.Close()
			// A noncooperative Open can finish late. Its returned resource is still owned
			// and must be aborted, never promoted to a successful receive.
			if mode == "open" {
				close(adapter.release)
			} else {
				defer close(adapter.release)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("late export leaked worker")
			}
			select {
			case <-adapter.aborted:
			default:
				t.Fatal("returned output not released")
			}
			if notified {
				t.Fatal("ended export notified success")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("stage not retired", err)
			}
		})
	}
}

func TestFileExportAcknowledgementWaitsForFinish(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	adapter := &blockingFileExport{mode: "finish", entered: make(chan struct{}), release: make(chan struct{}), aborted: make(chan struct{})}
	root := filepath.Join(t.TempDir(), "private-stage")
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
	if err := gate.decideExport(approvalRequest(t, gate), root, newFileExportSelection(adapter)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-adapter.entered:
	case <-time.After(time.Second):
		t.Fatal("finish not entered")
	}
	select {
	case err := <-sent:
		t.Fatal("sender completed before observed finish", err)
	default:
	}
	close(adapter.release)
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("successful export not acknowledged")
	}
	<-done
}

func TestFileExportStagingBudgetAndReleaseRealMux(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	root := filepath.Join(t.TempDir(), "private-stage")
	done := make(chan uint32, maxActiveFileExports+2)
	host.OnStream(func(s Stream) {
		defer func() { done <- s.ID }()
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
	})
	var streams []Stream
	for i := 0; i < maxActiveFileExports+1; i++ {
		s, err := client.Open(PriBulk, FileOffer{Name: "same.bin", Size: 0})
		if err != nil {
			t.Fatal(err)
		}
		req := approvalRequest(t, gate)
		if err := gate.decideExport(req, root, newFileExportSelection(&checkedFileExport{})); err != nil {
			t.Fatal(err)
		}
		f, err := s.Recv(time.Second)
		if err != nil {
			t.Fatal(err)
		}
		var accepted FileAccept
		if json.Unmarshal(f.Payload, &accepted) != nil || accepted.OK != (i < maxActiveFileExports) {
			t.Fatal("active stage cap not enforced", i, accepted)
		}
		if accepted.OK {
			streams = append(streams, s)
		} else {
			<-done
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != maxActiveFileExports {
		t.Fatal("wrong private stage count", entries, err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatal("stage directory not private")
		}
		files, err := os.ReadDir(filepath.Join(root, e.Name()))
		if err != nil || len(files) != 1 {
			t.Fatal("wrong staging contents")
		}
		st, err := files[0].Info()
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatal("stage file not private")
		}
	}
	for _, s := range streams {
		if err := client.send(s.ID, msgFin, PriBulk, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Recv(time.Second); err != nil {
			t.Fatal(err)
		}
		<-done
	}
	gate.exportBudget.mu.Lock()
	active, size := gate.exportBudget.active, gate.exportBudget.bytes
	gate.exportBudget.mu.Unlock()
	if active != 0 || size != 0 {
		t.Fatal("staging reservation leaked", active, size)
	}
	source := filepath.Join(t.TempDir(), "later.bin")
	os.WriteFile(source, []byte("later"), 0600)
	sent := make(chan error, 1)
	go func() { sent <- SendFile(client, source) }()
	if err := gate.decideExport(approvalRequest(t, gate), root, newFileExportSelection(&checkedFileExport{})); err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal("capacity never released", err)
	}
	<-done
}

func TestFileExportByteBudgetRejectsWithoutOverflow(t *testing.T) {
	var budget fileExportBudget
	if budget.take(-1) || budget.take(maxStagedFileExportBytes+1) {
		t.Fatal("invalid reservation accepted")
	}
	if !budget.take(maxStagedFileExportBytes) {
		t.Fatal("bounded reservation refused")
	}
	if budget.take(1) {
		t.Fatal("aggregate byte cap exceeded")
	}
	budget.release(maxStagedFileExportBytes)
	if !budget.take(1) {
		t.Fatal("byte capacity not returned")
	}
	budget.release(1)
}
