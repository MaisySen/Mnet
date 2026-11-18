package session

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileReceiverClosedBeforeStorage(t *testing.T) {
	for _, withAuthorization := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-extra-gate", true: "authorization"}[withAuthorization], func(t *testing.T) {
			a, b := net.Pipe()
			defer b.Close()
			host := newMux(a, StreamFile+1)
			host.Close()
			dir := filepath.Join(t.TempDir(), "not-created")
			var calls atomic.Int32
			receiver := FileReceiver{Mux: host, Dir: dir}
			if withAuthorization {
				receiver.Authorized = func() bool { calls.Add(1); return true }
			}
			offer, _ := json.Marshal(FileOffer{Name: "fixture.bin", Size: 0})
			receiver.Handle(Stream{Mux: host, ID: StreamFile, OpenJSON: offer})
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("closed session entered storage", err)
			}
			if calls.Load() != 0 {
				t.Fatal("closed session dispatched authorization")
			}
		})
	}
}

func TestFileReceiverCloseAtStorageBoundaries(t *testing.T) {
	// Calls name the existing boundaries, plus the new pre-sync boundary:
	// entry, pre-directory, pre-write, pre-sync, pre-publication, pre-notify.
	for _, tc := range []struct {
		name      string
		call      int
		published bool
	}{
		{"pre-directory", 2, false}, {"pre-write", 3, false},
		{"pre-sync", 4, false}, {"pre-publication", 5, false},
		{"pre-notify", 6, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			dir := filepath.Join(t.TempDir(), "received")
			var calls, notified atomic.Int32
			done := make(chan struct{})
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, Dir: dir, Authorized: func() bool {
					if int(calls.Add(1)) == tc.call {
						// A callback may return true even after its session was retired.
						// Ending here makes that ordering deterministic, without timing sleeps.
						host.Close()
					}
					return true
				}, OnDone: func(string, int64) { notified.Add(1) }}.Handle(s)
			})
			stream, err := client.Open(PriBulk, FileOffer{Name: "fixture.bin", Size: 3})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.release()
			if tc.call > 2 {
				frame, err := stream.Recv(time.Second)
				if err != nil {
					t.Fatal(err)
				}
				var accept FileAccept
				if err := json.Unmarshal(frame.Payload, &accept); err != nil || !accept.OK {
					t.Fatal("not accepted", err)
				}
				// These sends can observe the peer close after the chosen boundary.
				_ = client.SendBytes(stream.ID, PriBulk, []byte("abc"))
				_ = client.send(stream.ID, msgFin, PriBulk, nil)
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("retired receiver did not return")
			}
			if int(calls.Load()) != tc.call {
				t.Fatal("work continued across terminal boundary", calls.Load())
			}
			if notified.Load() != 0 {
				t.Fatal("terminal session completion notified")
			}
			target := filepath.Join(dir, "fixture.bin")
			data, err := os.ReadFile(target)
			if tc.published {
				if err != nil || string(data) != "abc" {
					t.Fatal("already-published file changed", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("terminal session published file", err)
			}
			if tc.call == 2 {
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatal("late authorization created directory", err)
				}
			} else {
				entries, err := os.ReadDir(dir)
				expected := 0
				if tc.published {
					expected = 1
				}
				if err != nil || len(entries) != expected {
					t.Fatal("temporary data retained", entries, err)
				}
			}
		})
	}
}
