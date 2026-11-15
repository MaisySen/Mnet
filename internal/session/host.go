package session

import (
	"encoding/json"
	"sync"
	"time"
)

// Capturer produces host screen frames. Platform implementations:
// Android MediaProjection, Linux X11.
type Capturer interface {
	// Capture returns one encoded frame (JPEG in v1) and whether the
	// screen changed since the last call.
	Capture() (img []byte, changed bool, err error)
	Geometry() (width, height int)
}

// Injector applies viewer input on the host. Platform
// implementations: Android accessibility, Linux XTEST.
type Injector interface {
	Inject(ev InputEvent) error
}

// Host runs the hosting side of one session: it streams the screen,
// applies input, relays chat and receives files. Handlers and the
// incoming-file directory can be set or changed at any time via the
// Set methods; they take effect on the next message or stream.
type Host struct {
	Mux      *Mux
	Cap      Capturer
	Inj      Injector
	FPS      int
	Incoming FileReceiver

	hmu    sync.RWMutex
	onChat func(ChatMessage)
	onFile func(name string, size int64)

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// SetOnChat sets the chat handler.
func (h *Host) SetOnChat(f func(ChatMessage)) {
	h.hmu.Lock()
	defer h.hmu.Unlock()
	h.onChat = f
}

// SetOnFile sets the received-file handler.
func (h *Host) SetOnFile(f func(name string, size int64)) {
	h.hmu.Lock()
	defer h.hmu.Unlock()
	h.onFile = f
}

// SetFileDir sets where incoming files land.
func (h *Host) SetFileDir(dir string) {
	h.hmu.Lock()
	defer h.hmu.Unlock()
	h.Incoming.Dir = dir
}

// ServeHost wires a mux (from Listen) to host duties and starts the
// screen loop. fps <= 0 defaults to 5.
func ServeHost(m *Mux, cap Capturer, inj Injector, fps int) *Host {
	if fps <= 0 {
		fps = 5
	}
	h := &Host{
		Mux:  m,
		Cap:  cap,
		Inj:  inj,
		FPS:  fps,
		stop: make(chan struct{}),
	}
	h.Incoming = FileReceiver{Mux: m, OnDone: func(name string, size int64) {
		h.hmu.RLock()
		f := h.onFile
		h.hmu.RUnlock()
		if f != nil {
			f(name, size)
		}
	}}
	m.OnStream(h.handleStream)
	// Open the screen stream host-to-viewer with explicit geometry,
	// then mark it opened so screen frames don't trigger the
	// mux's implicit OPEN.
	w, hh := cap.Geometry()
	open := ScreenOpen{Codec: "jpeg", Width: w, Height: hh, FPS: fps}
	if err := m.sendJSON(StreamScreen, msgOpen, PriScreen, open); err == nil {
		m.wmu.Lock()
		m.opened[StreamScreen] = true
		m.wmu.Unlock()
		h.wg.Add(1)
		go h.screenLoop()
	}
	return h
}

func (h *Host) handleStream(s Stream) {
	switch {
	case s.ID == StreamInput:
		h.wg.Add(1)
		go h.inputLoop(s)
	case s.ID == StreamChat:
		h.wg.Add(1)
		go h.chatLoop(s)
	case s.ID >= StreamFile:
		h.wg.Add(1)
		go func() { defer h.wg.Done(); h.Incoming.Handle(s) }()
	}
}

// SendChat sends a chat line from the host side.
func (h *Host) SendChat(from, text string) error {
	return h.Mux.SendJSON(StreamChat, PriChat, ChatMessage{From: from, Text: text, At: time.Now().Unix()})
}

func (h *Host) screenLoop() {
	defer h.wg.Done()
	interval := time.Second / time.Duration(h.FPS)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var seq uint32
	for {
		select {
		case <-h.stop:
			return
		case <-h.Mux.closed:
			return
		case <-tick.C:
		}
		img, changed, err := h.Cap.Capture()
		if err != nil {
			return
		}
		if !changed {
			continue
		}
		seq++
		fr := ScreenFrame{Seq: seq, Full: true, Image: img}
		b, err := json.Marshal(fr)
		if err != nil {
			continue
		}
		if err := h.Mux.SendBytes(StreamScreen, PriScreen, b); err != nil {
			return
		}
	}
}

func (h *Host) inputLoop(s Stream) {
	defer h.wg.Done()
	for {
		f, err := s.Recv(0) // block until close
		if err != nil {
			return
		}
		var ev InputEvent
		if err := json.Unmarshal(f.Payload, &ev); err != nil {
			continue
		}
		if h.Inj != nil {
			h.Inj.Inject(ev)
		}
	}
}

func (h *Host) chatLoop(s Stream) {
	defer h.wg.Done()
	for {
		f, err := s.Recv(0)
		if err != nil {
			return
		}
		var m ChatMessage
		if err := json.Unmarshal(f.Payload, &m); err != nil {
			continue
		}
		h.hmu.RLock()
		cb := h.onChat
		h.hmu.RUnlock()
		if cb != nil {
			cb(m)
		}
	}
}

// Stop ends hosting and closes the session: the host side owns the
// connection it accepted.
func (h *Host) Stop() {
	h.stopOnce.Do(func() {
		close(h.stop)
		h.Mux.Close()
		h.wg.Wait()
	})
}
