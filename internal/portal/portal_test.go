package portal

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeCall struct {
	path   string
	iface  string
	method string
	args   []any
}

// fakeBus scripts one full negotiation: method calls in order, a
// Response per request-answerable call, and a direct reply for
// OpenPipeWireRemote.
type fakeBus struct {
	calls     []fakeCall
	responses []Response
	fdReply   []any
	reqNo     int
	closed    bool
}

func (f *fakeBus) Call(iface, method string, args ...any) ([]any, error) {
	return f.CallOn(rootPath, iface, method, args...)
}

func (f *fakeBus) CallOn(path, iface, method string, args ...any) ([]any, error) {
	f.calls = append(f.calls, fakeCall{path: path, iface: iface, method: method, args: args})
	if method == "OpenPipeWireRemote" {
		return f.fdReply, nil
	}
	f.reqNo++
	return []any{"/org/freedesktop/portal/request/1_" + itoa(f.reqNo)}, nil
}

func (f *fakeBus) WatchResponse(requestPath string) (<-chan Response, func(), error) {
	ch := make(chan Response, 1)
	if len(f.responses) > 0 {
		ch <- f.responses[0]
		f.responses = f.responses[1:]
	}
	return ch, func() {}, nil
}

func (f *fakeBus) Close() error { f.closed = true; return nil }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func okSession() Response {
	return Response{Code: 0, Results: map[string]any{"session_handle": "/org/freedesktop/portal/session/1_7/mnet"}}
}

func okStart() Response {
	return Response{Code: 0, Results: map[string]any{
		"streams": []any{
			[]any{uint32(42), map[string]any{"size": []any{int32(1920), int32(1080)}}},
		},
	}}
}

func TestOpenMonitorHappyPath(t *testing.T) {
	bus := &fakeBus{
		responses: []Response{okSession(), {Code: 0}, okStart()},
		fdReply:   []any{int32(7)},
	}
	c := NewClient(bus)
	c.ResponseTimeout = time.Second
	s, err := c.OpenMonitor()
	if err != nil {
		t.Fatalf("OpenMonitor: %v", err)
	}
	if s.NodeID != 42 || s.Width != 1920 || s.Height != 1080 || s.FD != 7 {
		t.Fatalf("stream = %+v", s)
	}
	if s.Session() == "" {
		t.Fatal("no session handle recorded")
	}
	wantMethods := []string{"CreateSession", "SelectSources", "Start", "OpenPipeWireRemote"}
	var got []string
	for _, call := range bus.calls {
		got = append(got, call.method)
	}
	if !reflect.DeepEqual(got, wantMethods) {
		t.Fatalf("methods = %v, want %v", got, wantMethods)
	}
	// SelectSources must ask for one monitor with the cursor burned in.
	sel := bus.calls[1]
	opts, ok := sel.args[1].(map[string]any)
	if !ok {
		t.Fatalf("SelectSources options shape %T", sel.args[1])
	}
	if opts["types"] != uint32(sourceMonitor) || opts["cursor_mode"] != uint32(cursorEmbedded) || opts["multiple"] != false {
		t.Fatalf("SelectSources options = %v", opts)
	}
	// The session handle from the response must scope later calls.
	if sel.args[0] != "/org/freedesktop/portal/session/1_7/mnet" {
		t.Fatalf("SelectSources session arg = %v", sel.args[0])
	}
	if err := s.Close(); err != nil || !bus.closed {
		t.Fatalf("Close: err=%v busClosed=%v", err, bus.closed)
	}
}

func TestOpenMonitorDismissed(t *testing.T) {
	bus := &fakeBus{
		responses: []Response{okSession(), {Code: 0}, {Code: 1}},
	}
	c := NewClient(bus)
	c.ResponseTimeout = time.Second
	_, err := c.OpenMonitor()
	if !errors.Is(err, ErrDismissed) {
		t.Fatalf("err = %v, want ErrDismissed", err)
	}
}

func TestOpenMonitorNoStreams(t *testing.T) {
	bus := &fakeBus{
		responses: []Response{okSession(), {Code: 0}, {Code: 0, Results: map[string]any{}}},
	}
	c := NewClient(bus)
	c.ResponseTimeout = time.Second
	_, err := c.OpenMonitor()
	if !errors.Is(err, ErrNoStreams) {
		t.Fatalf("err = %v, want ErrNoStreams", err)
	}
}

func TestOpenMonitorResponseTimeout(t *testing.T) {
	bus := &fakeBus{} // WatchResponse delivers nothing
	c := NewClient(bus)
	c.ResponseTimeout = 20 * time.Millisecond
	_, err := c.OpenMonitor()
	if err == nil || !strings.Contains(err.Error(), "no portal response") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

func TestOpenMonitorRestoreToken(t *testing.T) {
	start := okStart()
	start.Results["restore_token"] = "v2_token-abc"
	bus := &fakeBus{
		responses: []Response{okSession(), {Code: 0}, start},
		fdReply:   []any{int32(7)},
	}
	c := NewClient(bus)
	c.ResponseTimeout = time.Second
	c.RestoreToken = "v1_token-xyz"
	s, err := c.OpenMonitor()
	if err != nil {
		t.Fatalf("OpenMonitor: %v", err)
	}
	if s.RestoreToken != "v2_token-abc" {
		t.Fatalf("RestoreToken = %q", s.RestoreToken)
	}
	sel := bus.calls[1]
	opts := sel.args[1].(map[string]any)
	if opts["restore_token"] != "v1_token-xyz" {
		t.Fatalf("restore_token option = %v", opts["restore_token"])
	}
	if opts["persist_mode"] != uint32(2) {
		t.Fatalf("persist_mode = %v", opts["persist_mode"])
	}
}

func TestOpenMonitorNoRestoreTokenWithoutPersist(t *testing.T) {
	bus := &fakeBus{
		responses: []Response{okSession(), {Code: 0}, okStart()},
		fdReply:   []any{int32(7)},
	}
	c := NewClient(bus)
	c.ResponseTimeout = time.Second
	s, err := c.OpenMonitor()
	if err != nil {
		t.Fatalf("OpenMonitor: %v", err)
	}
	if s.RestoreToken != "" {
		t.Fatalf("unexpected RestoreToken %q", s.RestoreToken)
	}
	opts := bus.calls[1].args[1].(map[string]any)
	if _, present := opts["restore_token"]; present {
		t.Fatalf("restore_token sent without a stored token: %v", opts)
	}
}

func TestFirstStreamParsing(t *testing.T) {
	node, w, h, err := firstStream(okStart().Results)
	if err != nil || node != 42 || w != 1920 || h != 1080 {
		t.Fatalf("firstStream = %d %dx%d %v", node, w, h, err)
	}
	// Size absent: geometry unknown, still a valid stream.
	res := map[string]any{"streams": []any{[]any{uint32(9), map[string]any{}}}}
	node, w, h, err = firstStream(res)
	if err != nil || node != 9 || w != 0 || h != 0 {
		t.Fatalf("firstStream no-size = %d %dx%d %v", node, w, h, err)
	}
	if _, _, _, err := firstStream(map[string]any{}); !errors.Is(err, ErrNoStreams) {
		t.Fatalf("empty results err = %v", err)
	}
	if _, _, _, err := firstStream(map[string]any{"streams": []any{}}); !errors.Is(err, ErrNoStreams) {
		t.Fatalf("empty streams err = %v", err)
	}
	if _, _, _, err := firstStream(map[string]any{"streams": "junk"}); err == nil {
		t.Fatal("bad streams type accepted")
	}
}

func TestFDResult(t *testing.T) {
	if fd, ok := fdResult([]any{int32(5)}); !ok || fd != 5 {
		t.Fatalf("int32 fd = %d %v", fd, ok)
	}
	if _, ok := fdResult([]any{"nope"}); ok {
		t.Fatal("string accepted as fd")
	}
	if _, ok := fdResult(nil); ok {
		t.Fatal("empty reply accepted as fd")
	}
}

func TestPumpCommand(t *testing.T) {
	argv := PumpCommand(42, 3, 640, 360)
	joined := strings.Join(argv, " ")
	for _, want := range []string{"gst-launch-1.0", "pipewiresrc", "fd=3", "path=42", "video/x-raw,format=BGRA,width=640,height=360", "fdsink"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv %q missing %q", joined, want)
		}
	}
}

func TestPumpReadFrame(t *testing.T) {
	w, h := 4, 2
	frame := w * h * 4
	data := make([]byte, frame*2)
	for i := range data {
		data[i] = byte(i)
	}
	p := NewRawPump(bytes.NewReader(data), w, h)
	var reuse []byte
	for n := 0; n < 2; n++ {
		buf, err := p.ReadFrame(reuse)
		if err != nil {
			t.Fatalf("frame %d: %v", n, err)
		}
		if !bytes.Equal(buf, data[n*frame:(n+1)*frame]) {
			t.Fatalf("frame %d mismatch", n)
		}
		reuse = buf
	}
	if _, err := p.ReadFrame(reuse); err == nil {
		t.Fatal("third read beyond EOF succeeded")
	}
	// Short frame at the end must error, not return partial data.
	p = NewRawPump(bytes.NewReader(data[:frame-1]), w, h)
	if _, err := p.ReadFrame(nil); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short frame err = %v", err)
	}
}

func TestPumpCloseInterruptsRead(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	p := NewRawPump(r, 2, 2)
	result := make(chan error, 1)
	go func() { _, err := p.ReadFrame(nil); result <- err }()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("blocked read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not interrupt read")
	}
}
