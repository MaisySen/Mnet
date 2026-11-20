package desk

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type deskFragmentReader struct {
	source *bytes.Reader
	chunk  int
}

func (r deskFragmentReader) Read(p []byte) (int, error) {
	return r.source.Read(p[:min(len(p), r.chunk)])
}

type deskFragmentWriter struct {
	target  bytes.Buffer
	chunk   int
	failAt  int
	failure error
}

func (w *deskFragmentWriter) Write(p []byte) (int, error) {
	if w.failure != nil && w.target.Len() >= w.failAt {
		return 0, w.failure
	}
	n := min(len(p), w.chunk)
	if w.failure != nil {
		n = min(n, w.failAt-w.target.Len())
	}
	written, err := w.target.Write(p[:n])
	if err == nil && w.failure != nil && w.target.Len() >= w.failAt {
		return written, w.failure
	}
	return written, err
}

// Pure in-memory wire framing. No TLS, run, approval or native backend exists.
func TestDeskWireFragmentationBoundariesAndJoinedErrors(t *testing.T) {
	binding := Binding{Run: strings.Repeat("a", 64), Primary: strings.Repeat("b", 64), Recipient: strings.Repeat("c", 64), Layout: strings.Repeat("d", 64), Keyboard: strings.Repeat("e", 64), Display: "target"}
	messages := []wireMessage{{Kind: "hello", Binding: &binding}, {Kind: "ready", OK: true}, {Kind: "input", Input: &Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}}, {Kind: "pulse"}, {Kind: "stop", OK: true}}
	var stream []byte
	var frames [][]byte
	for _, message := range messages {
		body, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		frame := make([]byte, 4+len(body))
		binary.BigEndian.PutUint32(frame, uint32(len(body)))
		copy(frame[4:], body)
		frames = append(frames, frame)
		stream = append(stream, frame...)
	}
	for _, chunk := range []int{1, 2, 3, 4, 7, 31, 4096} {
		writer := &deskFragmentWriter{chunk: chunk}
		if chunk < len(frames[0]) {
			if err := writeWire(writer, messages[0]); !errors.Is(err, io.ErrShortWrite) {
				t.Fatal("invalid short-without-error writer not refused", chunk, err)
			}
			if !bytes.Equal(writer.target.Bytes(), frames[0][:chunk]) {
				t.Fatal("partial output differs", chunk)
			}
		} else {
			for _, message := range messages {
				if err := writeWire(writer, message); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(writer.target.Bytes(), stream) {
				t.Fatal("wire bytes differ from independent framing", chunk)
			}
		}
		source := bytes.NewReader(stream)
		reader := deskFragmentReader{source, chunk}
		consumed := 0
		for i, message := range messages {
			got, err := readWire(reader)
			if err != nil || !reflect.DeepEqual(got, message) {
				t.Fatal("fragmented message changed", chunk, i, got, err)
			}
			consumed += len(frames[i])
			if source.Len() != len(stream)-consumed {
				t.Fatal("reader consumed a successor frame", chunk, i, source.Len())
			}
		}
		if _, err := readWire(reader); !errors.Is(err, io.EOF) {
			t.Fatal("clean stream end not retained", err)
		}
	}
	failure := errors.New("fixture writer failed")
	for i, message := range messages {
		for cut := 0; cut < len(frames[i]); cut++ {
			writer := &deskFragmentWriter{chunk: 4096, failAt: cut, failure: failure}
			if err := writeWire(writer, message); !errors.Is(err, failure) {
				t.Fatal("write failure lost", i, cut, err)
			}
			if !bytes.Equal(writer.target.Bytes(), frames[i][:cut]) {
				t.Fatal("failed write output changed", i, cut)
			}
			for _, chunk := range []int{1, 7} {
				source := bytes.NewReader(frames[i][:cut])
				if _, err := readWire(deskFragmentReader{source, chunk}); err == nil || !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatal("truncated frame accepted or error lost", i, cut, chunk, err)
				}
			}
		}
	}
}
