package session

import (
	"bytes"
	"encoding/json"
	"image/color"
	"image/jpeg"
	"sync"
	"testing"
	"time"
)

// Replacing a render handler or closing a transport while a callback is
// blocked must not leave the pending image bound to the old handler.
func TestFrameCallbackPendingLifecycle(t *testing.T) {
	for _, closing := range []bool{false, true} {
		name := "replace-handler"
		if closing {
			name = "close-mux"
		}
		t.Run(name, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			entered := make(chan struct{})
			release := make(chan struct{})
			returned := make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			oldCalls := make(chan uint32, 4)
			newCalls := make(chan uint32, 4)
			viewer := &Viewer{onFrame: func(fr ScreenFrame) {
				if fr.Seq == 1 {
					close(entered)
					<-release
					close(returned)
				}
				oldCalls <- fr.Seq
			}}
			incoming := make(chan Frame)
			decoded := make(chan struct{})
			go func() {
				viewer.screenLoop(Stream{Mux: client, in: incoming})
				close(decoded)
			}()
			var encoded bytes.Buffer
			if err := jpeg.Encode(&encoded, solid(16, 16, color.RGBA{R: 90, A: 255}), nil); err != nil {
				t.Fatal(err)
			}
			send := func(seq uint32) {
				t.Helper()
				payload, err := json.Marshal(ScreenFrame{Seq: seq, Full: true, Image: encoded.Bytes()})
				if err != nil {
					t.Fatal(err)
				}
				select {
				case incoming <- Frame{Type: msgData, Payload: payload}:
				case <-time.After(time.Second):
					t.Fatal("decoder blocked")
				}
			}
			send(1)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("callback not entered")
			}
			send(2)
			close(incoming)
			select {
			case <-decoded:
			case <-time.After(time.Second):
				t.Fatal("decoder did not finish")
			}
			viewer.SetOnFrame(func(fr ScreenFrame) { newCalls <- fr.Seq })
			if closing {
				client.Close()
			}
			once.Do(func() { close(release) })
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("running callback did not resume")
			}
			select {
			case seq := <-oldCalls:
				if seq != 1 {
					t.Fatalf("old handler got sequence %d", seq)
				}
			case <-time.After(time.Second):
				t.Fatal("initial callback missing")
			}
			if !closing {
				select {
				case seq := <-newCalls:
					if seq != 2 {
						t.Fatalf("replacement got sequence %d", seq)
					}
				case <-time.After(time.Second):
					t.Fatal("pending image did not use replacement handler")
				}
			}
			select {
			case seq := <-oldCalls:
				t.Fatalf("pending image reached stale handler: %d", seq)
			case seq := <-newCalls:
				t.Fatalf("unexpected pending callback: %d", seq)
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}
