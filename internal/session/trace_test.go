package session

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestFrameTraceBoundedMetadataOnly(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	m := newMux(a, StreamFile)
	m.traceEnabled = true
	for i := 0; i < len(m.traceEvents)+20; i++ {
		m.traceFrame("test", Frame{StreamID: uint32(i), Type: msgData, Payload: []byte("DO-NOT-RETAIN-PAYLOAD")})
	}
	if m.traceNext != uint64(len(m.traceEvents)+20) {
		t.Fatal(m.traceNext)
	}
	for _, e := range m.traceEvents {
		if e.id < 20 || e.stage != "test" || e.size != 21 || e.at.IsZero() {
			t.Fatalf("bad event: %+v", e)
		}
		if strings.Contains(fmt.Sprint(e), "DO-NOT-RETAIN-PAYLOAD") {
			t.Fatal("trace retains payload")
		}
	}
	// Tracing must not queue frames on the actual connection.
	if m.queuedBytes != 0 || len(m.pending[PriBulk]) != 0 {
		t.Fatal("trace changed wire queue")
	}
}

func TestTraceDisabledDoesNotRecord(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	m := newMux(a, StreamFile)
	m.traceEnabled = false
	m.traceFrame("test", Frame{Payload: bytes.Repeat([]byte("x"), 100)})
	if m.traceNext != 0 {
		t.Fatal("disabled trace recorded an event")
	}
}

func TestTraceSettingCapturedPerMux(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	t.Setenv("MNET_SESSION_TRACE", "1")
	enabled := newMux(a, StreamFile)
	t.Setenv("MNET_SESSION_TRACE", "0")
	disabled := newMux(b, StreamFile)
	enabled.traceFrame("test", Frame{Type: msgData})
	disabled.traceFrame("test", Frame{Type: msgData})
	if enabled.traceNext != 1 || disabled.traceNext != 0 {
		t.Fatal("trace setting not captured per session")
	}
}
