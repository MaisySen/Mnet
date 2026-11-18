package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type controlledFileReader struct {
	reads int
	read  func([]byte) (int, error)
}

func (r *controlledFileReader) Read(p []byte) (int, error) { r.reads++; return r.read(p) }
func TestOwnedFileCopyCancellationAndExactSize(t *testing.T) {
	closed := make(chan struct{})
	src := bytes.NewBufferString("original-appended")
	var dst bytes.Buffer
	if err := copyOwnedFileBytes(closed, &dst, src, 8); err != nil {
		t.Fatal(err)
	}
	if dst.String() != "original" || src.String() != "-appended" {
		t.Fatal("read beyond snapshot", dst.String(), src.String())
	}
	close(closed)
	reader := &controlledFileReader{read: func([]byte) (int, error) { t.Fatal("read after close"); return 0, nil }}
	if err := copyOwnedFileBytes(closed, &dst, reader, 1); !errors.Is(err, net.ErrClosed) {
		t.Fatal("canceled copy accepted", err)
	}
	if reader.reads != 0 {
		t.Fatal("closed copy entered file read")
	}
	if err := copyOwnedFileBytes(make(chan struct{}), io.Discard, bytes.NewBufferString("x"), 2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("shrink not detected", err)
	}
	if err := copyOwnedFileBytes(make(chan struct{}), io.Discard, &controlledFileReader{read: func([]byte) (int, error) { return 0, nil }}, 1); !errors.Is(err, io.ErrNoProgress) {
		t.Fatal("zero reader looped", err)
	}
}
func TestOwnedFileCopyCloseDuringReadRefusesLateBytes(t *testing.T) {
	entered, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	src := &controlledFileReader{read: func(p []byte) (int, error) { close(entered); <-release; return copy(p, []byte{1}), nil }}
	var dst bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- copyOwnedFileBytes(closed, &dst, src, 1) }()
	<-entered
	close(closed)
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatal("late bytes accepted", err)
		}
	case <-time.After(time.Second):
		t.Fatal("copy did not return")
	}
	if dst.Len() != 0 {
		t.Fatal("late data forwarded after observed close")
	}
}
func TestSendFileSourceMutationNeverSendsAppendedBytes(t *testing.T) {
	for _, kind := range []string{"grow", "shrink", "same"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.bin")
			original := []byte("original")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			a, b := net.Pipe()
			client := newMux(a, StreamFile)
			server := newMux(b, StreamFile+1)
			defer client.Close()
			defer server.Close()
			client.start()
			server.start()
			observed := make(chan []byte, 1)
			offered := make(chan FileOffer, 1)
			server.OnStream(func(stream Stream) {
				defer stream.release()
				var offer FileOffer
				if json.Unmarshal(stream.OpenJSON, &offer) != nil {
					return
				}
				offered <- offer
				changed := []byte("original-sensitive-appended")
				if kind == "shrink" {
					changed = []byte("x")
				}
				if kind == "same" {
					changed = []byte("mutated!")
				}
				if err := os.WriteFile(path, changed, 0600); err != nil {
					return
				}
				if err := server.SendJSON(stream.ID, PriBulk, FileAccept{OK: true}); err != nil {
					return
				}
				var data []byte
				for {
					frame, err := stream.Recv(time.Second)
					if err != nil {
						if kind == "same" {
							server.SendJSON(stream.ID, PriBulk, FileAck{OK: false})
						}
						observed <- data
						return
					}
					if frame.Type == msgRst {
						observed <- data
						return
					}
					data = append(data, frame.Payload...)
				}
			})
			if err := SendFile(client, path); err == nil {
				t.Fatal("mutated file reported sent")
			}
			offer := <-offered
			sum := sha256.Sum256(original)
			if offer.Size != int64(len(original)) || offer.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatal("offer changed with source")
			}
			select {
			case data := <-observed:
				if len(data) > len(original) {
					t.Fatal("appended data escaped offered snapshot", string(data))
				}
			case <-time.After(2 * time.Second):
				t.Fatal("failure did not reset stream")
			}
		})
	}
}
