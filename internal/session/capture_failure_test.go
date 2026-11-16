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
