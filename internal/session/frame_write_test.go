package session

import (
	"errors"
	"io"
	"testing"
)

type shortFrameWriter struct {
	calls, shortAt int
	err            error
}

func (w *shortFrameWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.shortAt {
		return len(p) - 1, w.err
	}
	return len(p), nil
}

func TestFrameWriteRefusesSilentShortHeaderOrPayload(t *testing.T) {
	for _, at := range []int{1, 2} {
		writer := &shortFrameWriter{shortAt: at}
		if err := writeFrame(writer, Frame{Payload: []byte("payload")}); !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("write%d: %v", at, err)
		}
		if writer.calls != at {
			t.Fatal("continued after truncated frame write")
		}
	}
}
func TestFrameWritePreservesTransportError(t *testing.T) {
	expected := errors.New("transport failed")
	for _, at := range []int{1, 2} {
		writer := &shortFrameWriter{shortAt: at, err: expected}
		if err := writeFrame(writer, Frame{Payload: []byte("payload")}); !errors.Is(err, expected) {
			t.Fatalf("write%d: %v", at, err)
		}
	}
}
