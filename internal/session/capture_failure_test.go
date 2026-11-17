package session

import (
	"errors"
	"testing"
	"time"
)

type failedCapturer struct{}

func (failedCapturer) Geometry() (int, int) { return 16, 16 }
func (failedCapturer) Capture() ([]byte, bool, error) {
	return nil, false, errors.New("capture failed")
}

type failedDeltaCapturer struct {
	failedCapturer
	fallback bool
}

func (c failedDeltaCapturer) CaptureDelta() ([]byte, []Tile, error) {
	if c.fallback {
		return nil, []Tile{{Rect: Rect{W: 8, H: 8}}}, nil
	}
	return nil, nil, errors.New("delta capture failed")
}

func TestHostCaptureFailureClosesSession(t *testing.T) {
	for _, tc := range []struct {
		name string
		cap  Capturer
	}{
		{"full", failedCapturer{}},
		{"delta", failedDeltaCapturer{}},
		{"keyframe-fallback", failedDeltaCapturer{fallback: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			ConnectViewer(client)
			h := ServeHost(host, tc.cap, nil, 100)
			defer h.Stop()
			select {
			case <-client.Closed():
			case <-time.After(time.Second):
				t.Fatal("capture failure left viewer connected to frozen session")
			}
			select {
			case <-host.Closed():
			case <-time.After(time.Second):
				t.Fatal("host transport remained open")
			}
		})
	}
}

type invalidGeometryCapturer struct{ width, height int }

func (c invalidGeometryCapturer) Geometry() (int, int)         { return c.width, c.height }
func (invalidGeometryCapturer) Capture() ([]byte, bool, error) { return nil, false, nil }
func TestHostRejectsInvalidCaptureGeometryBeforeStartingWorkers(t *testing.T) {
	for _, capture := range []Capturer{nil, invalidGeometryCapturer{0, 16}, invalidGeometryCapturer{-1, 16}, invalidGeometryCapturer{8193, 1}, invalidGeometryCapturer{8192, 8192}} {
		client, host := pipe(t)
		h := ServeHost(host, capture, nil, 8)
		select {
		case <-client.Closed():
		case <-time.After(time.Second):
			t.Fatal("invalid geometry left session open")
		}
		h.Stop()
		client.Close()
		host.Close()
	}
}
func TestHostFrameRateCannotCreateZeroTickerInterval(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	h := ServeHost(host, invalidGeometryCapturer{16, 16}, nil, int(^uint(0)>>1))
	defer h.Stop()
	if h.FPS != 240 {
		t.Fatal("host frame rate unbounded")
	}
}
