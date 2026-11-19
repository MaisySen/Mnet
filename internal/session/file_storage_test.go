package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

type checkedReceiveTransaction struct {
	bytes.Buffer
	events     []string
	fail       string
	onFinalize func()
}

func (s *checkedReceiveTransaction) Write(p []byte) (int, error) {
	s.events = append(s.events, "write")
	if s.fail == "write" {
		return 0, errors.New("private storage detail")
	}
	if s.fail == "short" {
		return len(p) - 1, nil
	}
	return s.Buffer.Write(p)
}
func (s *checkedReceiveTransaction) Finalize() error {
	s.events = append(s.events, "finalize")
	if s.onFinalize != nil {
		s.onFinalize()
	}
	if s.fail == "finalize" {
		return errors.New("private storage detail")
	}
	return nil
}
func (s *checkedReceiveTransaction) Publish() error {
	s.events = append(s.events, "publish")
	if s.fail == "publish" {
		return errors.New("private storage detail")
	}
	return nil
}
func (s *checkedReceiveTransaction) Abort() { s.events = append(s.events, "abort") }

func TestFileStorageTransactionFailureOwnership(t *testing.T) {
	for _, mode := range []string{"success", "write", "short", "digest", "finalize", "publish"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			storage := &checkedReceiveTransaction{fail: mode}
			done := make(chan struct{})
			notified := false
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, Dir: "explicit-local-directory", beginStorage: func(dir, name string) (receiveTransaction, error) {
					if dir != "explicit-local-directory" || name != "safe.bin" {
						t.Error("storage binding changed")
					}
					storage.events = append(storage.events, "begin")
					return storage, nil
				}, OnDone: func(string, int64) { notified = true }}.Handle(s)
			})
			offer := FileOffer{Name: "safe.bin", Size: 3}
			if mode == "digest" {
				offer.SHA256 = string(bytes.Repeat([]byte("0"), 64))
			}
			s, err := client.Open(PriBulk, offer)
			if err != nil {
				t.Fatal(err)
			}
			f, err := s.Recv(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var accepted FileAccept
			if json.Unmarshal(f.Payload, &accepted) != nil || !accepted.OK {
				t.Fatal("valid offer refused")
			}
			if err = client.SendBytes(s.ID, PriBulk, []byte("abc")); err != nil {
				t.Fatal(err)
			}
			if mode != "write" && mode != "short" {
				if err = client.send(s.ID, msgFin, PriBulk, nil); err != nil {
					t.Fatal(err)
				}
			}
			f, err = s.Recv(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var ack FileAck
			if json.Unmarshal(f.Payload, &ack) != nil || ack.OK != (mode == "success") {
				t.Fatal("wrong completion result", ack)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("storage not retired")
			}
			want := []string{"begin", "write"}
			if mode == "success" || mode == "finalize" || mode == "publish" {
				want = append(want, "finalize")
			}
			if mode == "success" || mode == "publish" {
				want = append(want, "publish")
			}
			want = append(want, "abort")
			if !reflect.DeepEqual(storage.events, want) {
				t.Fatal("transaction ordering", storage.events, want)
			}
			if notified != (mode == "success") {
				t.Fatal("failed storage notified completion")
			}
		})
	}
}

func TestPathReceiveTransactionKeepsPrivateNoOverwriteAndAbort(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "safe.bin")
	transaction, err := beginPathReceive(dir, "safe.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Abort()
	if transaction.Publish() == nil {
		t.Fatal("unfinalized bytes published")
	}
	if _, err = transaction.Write([]byte("verified")); err != nil {
		t.Fatal(err)
	}
	if err = transaction.Finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err = transaction.Write([]byte("late")); err == nil {
		t.Fatal("write after finalize accepted")
	}
	if transaction.Finalize() == nil {
		t.Fatal("double finalize accepted")
	}
	// Collision appears only after staging, so publication itself must refuse.
	if err = os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if transaction.Publish() == nil {
		t.Fatal("late collision overwritten")
	}
	transaction.Abort()
	transaction.Abort()
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep" {
		t.Fatal("abort changed existing file")
	}
	temps, _ := filepath.Glob(filepath.Join(dir, ".mnet-transfer-*"))
	if len(temps) != 0 {
		t.Fatal("staging leaked")
	}
	fresh, err := beginPathReceive(dir, "fresh.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Abort()
	fresh.Write([]byte("verified"))
	if err = fresh.Finalize(); err != nil {
		t.Fatal(err)
	}
	if err = fresh.Publish(); err != nil {
		t.Fatal(err)
	}
	if fresh.Publish() == nil {
		t.Fatal("publication repeated")
	}
	fresh.Abort()
	got, err = os.ReadFile(filepath.Join(dir, "fresh.bin"))
	if err != nil || string(got) != "verified" {
		t.Fatal("abort removed final output")
	}
	st, err := os.Stat(filepath.Join(dir, "fresh.bin"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("private file mode lost")
	}
}

func TestFileStorageTransactionCannotPublishAfterFinalizeEndsSession(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	storage := &checkedReceiveTransaction{onFinalize: func() { host.Close() }}
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, Dir: "explicit-local-directory", beginStorage: func(string, string) (receiveTransaction, error) { return storage, nil }}.Handle(s)
	})
	s, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Recv(time.Second); err != nil {
		t.Fatal(err)
	}
	if err = client.send(s.ID, msgFin, PriBulk, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("transaction not retired")
	}
	if !reflect.DeepEqual(storage.events, []string{"finalize", "abort"}) {
		t.Fatal("ended transaction published", storage.events)
	}
}

func TestFileStorageTransactionBeginsOnlyAfterCurrentApproval(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	var begins atomic.Int32
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate, beginStorage: func(string, string) (receiveTransaction, error) {
			begins.Add(1)
			return nil, errors.New("must not open")
		}}.Handle(s)
	})
	if _, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 0}); err != nil {
		t.Fatal(err)
	}
	req := approvalRequest(t, gate)
	if begins.Load() != 0 {
		t.Fatal("storage before local answer")
	}
	if err := gate.Decide(req, false, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("declined transaction not retired")
	}
	if begins.Load() != 0 {
		t.Fatal("declined local answer entered storage")
	}
}
