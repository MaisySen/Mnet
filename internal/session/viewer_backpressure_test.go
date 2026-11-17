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

// A blocked render callback must not fill the wire screen queue and prevent
// unrelated chat from being dispatched. Resolve every delta before coalescing
// completed display frames so later callbacks still see the entire image.
func TestBlockedFrameCallbackKeepsChatAndDeltaProgress(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	frames := make(chan ScreenFrame, 128)
	chats := make(chan ChatMessage, 1)
	ConnectViewerWithOptions(client, ViewerOptions{
		OnFrame: func(fr ScreenFrame) {
			if fr.Seq == 1 {
				close(entered)
				<-release
			}
			frames <- fr
		},
		OnChat: func(cm ChatMessage) { chats <- cm },
	})
	base := solid(16, 16, color.RGBA{R: 90, A: 255})
	var full bytes.Buffer
	if err := jpeg.Encode(&full, base, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	if err := host.SendJSON(StreamScreen, PriScreen, ScreenFrame{Seq: 1, Full: true, Image: full.Bytes()}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first callback not entered")
	}
	for seq := uint32(2); seq <= 64; seq++ {
		// Disjoint left/right tiles require both earlier and later deltas.
		x := 0
		c := color.RGBA{G: 230, A: 255}
		if seq%2 == 0 {
			x = 8
			c = color.RGBA{B: 230, A: 255}
		}
		tile := solid(8, 16, c)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, tile, &jpeg.Options{Quality: 100}); err != nil {
			t.Fatal(err)
		}
		fr := ScreenFrame{Seq: seq, Tiles: []Tile{{Rect: Rect{X: x, Y: 0, W: 8, H: 16}, Image: encoded.Bytes()}}}
		if err := host.SendJSON(StreamScreen, PriScreen, fr); err != nil {
			t.Fatal(err)
		}
	}
	// Same outbound priority makes the chat marker follow every screen frame.
	if err := host.SendJSON(StreamChat, PriScreen, ChatMessage{Text: "after-screen"}); err != nil {
		t.Fatal(err)
	}
	select {
	case cm := <-chats:
		if cm.Text != "after-screen" {
			t.Fatal(cm)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked screen callback stalled unrelated chat")
	}
	once.Do(func() { close(release) })
	deadline := time.After(3 * time.Second)
	for {
		select {
		case fr := <-frames:
			if fr.Seq != 64 {
				continue
			}
			if !fr.Full || len(fr.Tiles) != 0 {
				t.Fatal("callback did not receive a resolved full frame")
			}
			img, err := jpeg.Decode(bytes.NewReader(fr.Image))
			if err != nil {
				t.Fatal(err)
			}
			_, g, _, _ := img.At(2, 8).RGBA()
			_, _, b, _ := img.At(13, 8).RGBA()
			if g>>8 < 200 || b>>8 < 200 {
				t.Fatalf("delta chain broken: green=%d blue=%d", g>>8, b>>8)
			}
			return
		case <-deadline:
			t.Fatal("latest complete frame not delivered after callback resumed")
		}
	}
}

// A long-blocked consumer retains only the newest completed display image,
// including when FIN ends the screen stream before the callback can resume.
func TestFrameCallbackCoalescesPendingImagesAtStreamEnd(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	got := make(chan uint32, 128)
	viewer := &Viewer{onFrame: func(fr ScreenFrame) {
		if fr.Seq == 1 {
			close(entered)
			<-release
		}
		got <- fr.Seq
	}}
	incoming := make(chan Frame)
	decoded := make(chan struct{})
	go func() { viewer.screenLoop(Stream{Mux: client, in: incoming}); close(decoded) }()
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, solid(16, 16, color.RGBA{R: 90, A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	img := encoded.Bytes()
	send := func(seq uint32) {
		payload, err := json.Marshal(ScreenFrame{Seq: seq, Full: true, Image: img})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case incoming <- Frame{Type: msgData, Payload: payload}:
		case <-time.After(2 * time.Second):
			t.Fatal("decoder blocked on display callback")
		}
	}
	send(1)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback not entered")
	}
	for seq := uint32(2); seq <= 100; seq++ {
		send(seq)
	}
	close(incoming)
	select {
	case <-decoded:
	case <-time.After(time.Second):
		t.Fatal("screen decoder did not finish")
	}
	once.Do(func() { close(release) })
	for _, want := range []uint32{1, 100} {
		select {
		case seq := <-got:
			if seq != want {
				t.Fatalf("callback sequence=%d want%d", seq, want)
			}
		case <-time.After(time.Second):
			t.Fatal("completed callback image lost")
		}
	}
	select {
	case seq := <-got:
		t.Fatalf("obsolete display image delivered: %d", seq)
	case <-time.After(20 * time.Millisecond):
	}
}
