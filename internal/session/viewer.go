package session

import (
	"encoding/json"
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
func ConnectViewer(m *Mux) *Viewer {
	v := &Viewer{Mux: m}
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
		v.vmu.RLock()
		cb := v.onFrame
		v.vmu.RUnlock()
		if cb != nil {
			cb(fr)
		}
	}
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
