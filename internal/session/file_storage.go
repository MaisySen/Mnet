package session

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// receiveTransaction owns one private staging write and final publication.
// It is internal, not a native destination API. A future provider adapter must
// establish its own completion/rollback contract instead of assuming os.Link.
// Methods are called by one receiver worker. Abort always runs, including after
// successful Publish, and must release staging without deleting final output.
type receiveTransaction interface {
	io.Writer
	Finalize() error
	Publish() error
	Abort()
}

var errReceiveDestination = errors.New("destination already exists or is inaccessible")

type pathReceiveTransaction struct {
	file      *os.File
	temp      string
	target    string
	finalized bool
	published bool
}

func beginPathReceive(dir, name string) (receiveTransaction, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	target := filepath.Join(dir, name)
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return nil, errReceiveDestination
	}
	f, err := os.CreateTemp(dir, ".mnet-transfer-*")
	if err != nil {
		return nil, err
	}
	return &pathReceiveTransaction{file: f, temp: f.Name(), target: target}, nil
}
func (t *pathReceiveTransaction) Write(p []byte) (int, error) {
	if t.finalized {
		return 0, os.ErrClosed
	}
	return t.file.Write(p)
}
func (t *pathReceiveTransaction) Finalize() error {
	if t.finalized {
		return os.ErrClosed
	}
	if err := t.file.Sync(); err != nil {
		return err
	}
	if err := t.file.Close(); err != nil {
		return err
	}
	t.finalized = true
	return nil
}
func (t *pathReceiveTransaction) Publish() error {
	if !t.finalized || t.published {
		return os.ErrInvalid
	}
	// Atomic no-overwrite publication, unchanged from the path receiver.
	if err := os.Link(t.temp, t.target); err != nil {
		return err
	}
	t.published = true
	return nil
}
func (t *pathReceiveTransaction) Abort() {
	_ = t.file.Close()
	_ = os.Remove(t.temp)
}
