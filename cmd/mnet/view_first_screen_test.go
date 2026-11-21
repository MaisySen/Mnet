package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
)

func TestDesktopFirstScreenDeadlineRetiresSilentAdmittedTransport(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		if consumeSessionHello(b) != nil {
			return
		}
		payload, _ := json.Marshal(session.HelloReply{Version: 1, Accepted: true})
		var header [12]byte
		header[4] = 2
		binary.BigEndian.PutUint32(header[8:], uint32(len(payload)))
		if _, err := b.Write(header[:]); err != nil {
			return
		}
		if _, err := b.Write(payload); err != nil {
			return
		}
		io.Copy(io.Discard, b)
	}()
	mux, err := session.Dial(a, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	view := &testDesktopView{}
	calls := 0
	var viewed bool
	viewed, err = runDesktopWindowWithFirstScreenWait(context.Background(), view, func(context.Context) (*session.Mux, error) { calls++; return mux, nil }, false, 10*time.Millisecond)
	if !errors.Is(err, errDesktopFirstScreen) || calls != 1 || !view.closed || view.frames != 0 || viewed {
		t.Fatal("silent admitted session outcome", err, calls, view.closed, view.frames)
	}
	select {
	case <-mux.Closed():
	default:
		t.Fatal("timed out transport retained")
	}
	select {
	case <-peerDone:
	case <-time.After(time.Second):
		t.Fatal("peer did not observe close")
	}
}

func TestDesktopFirstScreenBoundDoesNotTimeOutHostConsent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	view := &testDesktopView{}
	dialEntered := make(chan struct{})
	timer := time.AfterFunc(30*time.Millisecond, cancel)
	defer timer.Stop()
	viewed, err := runDesktopWindowWithFirstScreenWait(ctx, view, func(ctx context.Context) (*session.Mux, error) {
		close(dialEntered)
		<-ctx.Done()
		return nil, ctx.Err()
	}, false, time.Nanosecond)
	if err != nil || !view.closed || view.frames != 0 || viewed {
		t.Fatal("first-screen deadline affected pending consent", err)
	}
	<-dialEntered
}

func TestHomeArgumentsDoNotInferLayoutOrAcceptExtraModes(t *testing.T) {
	for _, args := range [][]string{{"--layout"}, {"--layout", ""}, {"--layout", "a", "extra"}, {"--connect", "peer"}} {
		if _, err := homeArgs(args); err == nil {
			t.Fatal("invalid home option accepted", args)
		}
	}
	if path, err := homeArgs(nil); err != nil || path != (homeConfig{}) {
		t.Fatal(path, err)
	}
	if path, err := homeArgs([]string{"--layout", "chosen.json"}); err != nil || path.LayoutPath != "chosen.json" {
		t.Fatal(path, err)
	}
	if err := runHome(nil, nil, "", homeConfig{}); err == nil {
		t.Fatal("missing home owner accepted")
	}
}

// One complete frame is enough to finish startup. An unchanged desktop must
// not expire later merely because there is no new image to deliver.
type onceScreenCapture struct {
	data []byte
	sent bool
}

func (c *onceScreenCapture) Geometry() (int, int) { return 2, 2 }
func (c *onceScreenCapture) Capture() ([]byte, bool, error) {
	if c.sent {
		return c.data, false, nil
	}
	c.sent = true
	return c.data, true, nil
}

type firstScreenEndView struct {
	testDesktopView
	scheduled     sync.Once
	endAfterFrame chan struct{}
}

func (v *firstScreenEndView) Frame(fr session.ScreenFrame) error {
	if err := v.testDesktopView.Frame(fr); err != nil {
		return err
	}
	v.scheduled.Do(func() { time.AfterFunc(time.Second, func() { close(v.endAfterFrame) }) })
	return nil
}
func (v *firstScreenEndView) Poll() (bool, error) {
	select {
	case <-v.endAfterFrame:
		return true, nil
	default:
		return false, nil
	}
}
func TestDesktopFirstScreenCompletionDisarmsStartupBound(t *testing.T) {
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan *session.Mux, 1)
	go func() {
		host, _ := session.Listen(ln, "fixture", func(session.Hello, net.Addr) (bool, string) { return true, "" })
		accepted <- host
	}()
	right, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	mux, err := session.Dial(right, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	host := <-accepted
	if host == nil {
		t.Fatal("host unavailable")
	}
	defer host.Close()
	active := session.ServeHost(host, &onceScreenCapture{data: jpegData.Bytes()}, nil, 100)
	defer active.Stop()
	view := &firstScreenEndView{endAfterFrame: make(chan struct{})}
	viewed, err := runDesktopWindowWithFirstScreenWait(context.Background(), view, func(context.Context) (*session.Mux, error) { return mux, nil }, false, 500*time.Millisecond)
	if err != nil || view.frames != 1 || !view.closed || !viewed {
		t.Fatal("complete unchanged screen expired", err, view.frames, view.closed, viewed)
	}
}
