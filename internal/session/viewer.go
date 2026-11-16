package session

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"sync"
	"time"
)

// Viewer runs the viewing side of one session: it receives screen
// frames and chat, and sends input. Handlers can be attached or
// replaced at any time via the Set methods, which suits UI code that
// opens its screen after the session starts.
type Viewer struct {
	Mux *Mux

	vmu     sync.RWMutex
	onFrame func(ScreenFrame)
	onChat  func(ChatMessage)
	geo     ScreenOpen

	frameMu sync.Mutex
	base    *image.RGBA // last full frame, for delta compositing
}

// SetOnFrame sets the screen-frame handler.
func (v *Viewer) SetOnFrame(f func(ScreenFrame)) {
	v.vmu.Lock()
	defer v.vmu.Unlock()
	v.onFrame = f
}

// SetOnChat sets the chat handler.
func (v *Viewer) SetOnChat(f func(ChatMessage)) {
	v.vmu.Lock()
	defer v.vmu.Unlock()
	v.onChat = f
}

// ConnectViewer wires a mux (from Dial) to viewer duties. The host
// opens the screen stream after the handshake, so handlers must be
// registered before it arrives: ConnectViewer installs them and
// returns immediately.
// ViewerOptions installs callbacks before dispatching already queued streams.
type ViewerOptions struct {
	OnFrame func(ScreenFrame)
	OnChat  func(ChatMessage)
}

func ConnectViewer(m *Mux) *Viewer {
	return ConnectViewerWithOptions(m, ViewerOptions{})
}

func ConnectViewerWithOptions(m *Mux, opts ViewerOptions) *Viewer {
	m.Keepalive(15*time.Second, 45*time.Second)
	v := &Viewer{Mux: m, onFrame: opts.OnFrame, onChat: opts.OnChat}
	m.OnStream(v.handleStream)
	return v
}

// Geometry returns the host screen description once the screen stream
// has opened; nil before that.
func (v *Viewer) Geometry() *ScreenOpen {
	v.vmu.RLock()
	defer v.vmu.RUnlock()
	if v.geo.Codec == "" {
		return nil
	}
	g := v.geo
	return &g
}

func (v *Viewer) handleStream(s Stream) {
	switch {
	case s.ID == StreamScreen:
		v.vmu.Lock()
		json.Unmarshal(s.OpenJSON, &v.geo)
		v.vmu.Unlock()
		go v.screenLoop(s)
	case s.ID == StreamChat:
		go v.chatLoop(s)
	case s.ID >= StreamFile:
		// Viewers can receive files too (host-initiated send).
		fr := FileReceiver{Mux: v.Mux}
		go fr.Handle(s)
	}
}

// SendInput sends one input event to the host; the stream is opened
// implicitly on the first event.
func (v *Viewer) SendInput(ev InputEvent) error {
	return v.Mux.SendJSON(StreamInput, PriInteractive, ev)
}

// SendChat sends a chat line from the viewer side.
func (v *Viewer) SendChat(from, text string) error {
	return v.Mux.SendJSON(StreamChat, PriChat, ChatMessage{From: from, Text: text, At: time.Now().Unix()})
}

func (v *Viewer) screenLoop(s Stream) {
	for {
		f, err := s.Recv(0)
		if err != nil {
			return
		}
		var fr ScreenFrame
		if err := json.Unmarshal(f.Payload, &fr); err != nil {
			continue
		}
		if !v.resolveFrame(&fr) {
			continue // delta with no base yet: wait for a keyframe
		}
		v.vmu.RLock()
		cb := v.onFrame
		v.vmu.RUnlock()
		if cb != nil {
			cb(fr)
		}
	}
}

// resolveFrame turns delta frames into full frames: composites tiles
// onto the stored base and re-encodes, so OnFrame always delivers a
// complete JPEG. Returns false when the frame must be skipped.
func (v *Viewer) resolveFrame(fr *ScreenFrame) bool {
	v.frameMu.Lock()
	defer v.frameMu.Unlock()
	if fr.Full || len(fr.Tiles) == 0 {
		img, err := jpeg.Decode(bytes.NewReader(fr.Image))
		if err != nil {
			v.base = nil // Deltas after a broken keyframe cannot use an older base.
			return false
		}
		v.base = imageToRGBA(img)
		return true
	}
	if v.base == nil {
		return false
	}
	merged, err := CompositeTiles(v.base, fr.Tiles)
	if err != nil {
		v.base = nil // A missed delta also breaks the following delta chain.
		return false
	}
	v.base = merged
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, merged, &jpeg.Options{Quality: 80}); err != nil {
		return false
	}
	fr.Full = true
	fr.Image = buf.Bytes()
	fr.Tiles = nil
	return true
}

func imageToRGBA(src image.Image) *image.RGBA {
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

func (v *Viewer) chatLoop(s Stream) {
	for {
		f, err := s.Recv(0)
		if err != nil {
			return
		}
		var m ChatMessage
		if err := json.Unmarshal(f.Payload, &m); err != nil {
			continue
		}
		v.vmu.RLock()
		cb := v.onChat
		v.vmu.RUnlock()
		if cb != nil {
			cb(m)
		}
	}
}
