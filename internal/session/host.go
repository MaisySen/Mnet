package session

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// TuneableCapturer is a capturer whose encode quality can follow
// path conditions. SetQuality takes a JPEG quality 20-90.
type TuneableCapturer interface {
	SetQuality(q int)
}

// qualityForRTT picks the JPEG quality band for the measured path.
// Generous on good paths, frugal once the round trip says the link
// is the bottleneck.
func qualityForRTT(rtt time.Duration) int {
	switch {
	case rtt <= 0:
		return 70 // no measurement yet: host default
	case rtt > 250*time.Millisecond:
		return 40
	case rtt > 120*time.Millisecond:
		return 55
	default:
		return 70
	}
}

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

// DeltaCapturer is a Capturer that can send dirty-rect tile deltas
// instead of full frames. A nil full with empty tiles means no change.
type DeltaCapturer interface {
	Capturer
	CaptureDelta() (full []byte, tiles []Tile, err error)
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

	workerMu sync.Mutex
	hmu      sync.RWMutex
	onChat   func(ChatMessage)
	onFile   func(name string, size int64)

	rec      *Recorder // optional session recording tap
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

// SetRecorder taps the screen stream into a recording. Call before
// or soon after ServeHost starts; frames recorded from that point.
func (h *Host) SetRecorder(r *Recorder) {
	h.hmu.Lock()
	h.rec = r
	h.hmu.Unlock()
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
// HostOptions are installed before queued peer streams can be dispatched.
// Use these for startup configuration instead of racing the Set methods.
type HostOptions struct {
	FileDir  string
	OnChat   func(ChatMessage)
	OnFile   func(string, int64)
	Recorder *Recorder
}

func ServeHost(m *Mux, cap Capturer, inj Injector, fps int) *Host {
	return ServeHostWithOptions(m, cap, inj, fps, HostOptions{})
}

func ServeHostWithOptions(m *Mux, cap Capturer, inj Injector, fps int, opts HostOptions) *Host {
	if fps <= 0 {
		fps = 5
	}
	m.Keepalive(15*time.Second, 45*time.Second)
	h := &Host{
		Mux:  m,
		Cap:  cap,
		Inj:  inj,
		FPS:  fps,
		stop: make(chan struct{}),
	}
	h.onChat = opts.OnChat
	h.onFile = opts.OnFile
	h.rec = opts.Recorder
	h.Incoming = FileReceiver{Mux: m, Dir: opts.FileDir, OnDone: func(name string, size int64) {
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
	h.workerMu.Lock()
	defer h.workerMu.Unlock()
	select {
	case <-h.stop:
		return
	default:
	}
	switch {
	case s.ID == StreamInput:
		h.wg.Add(1)
		go h.inputLoop(s)
	case s.ID == StreamChat:
		h.wg.Add(1)
		go h.chatLoop(s)
	case s.ID >= StreamFile:
		h.wg.Add(1)
		h.hmu.RLock()
		receiver := h.Incoming
		h.hmu.RUnlock()
		go func() { defer h.wg.Done(); receiver.Handle(s) }()
	}
}

// SendChat sends a chat line from the host side.
func (h *Host) SendChat(from, text string) error {
	return h.Mux.SendJSON(StreamChat, PriChat, ChatMessage{From: from, Text: text, At: time.Now().Unix()})
}

func (h *Host) screenLoop() {
	// A hosted desktop session cannot remain healthy without screen production.
	// Close only the transport here; Stop owns capture cleanup and worker waits.
	defer h.Mux.Close()
	defer h.wg.Done()
	interval := time.Second / time.Duration(h.FPS)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	dc, _ := h.Cap.(DeltaCapturer)
	tune, _ := h.Cap.(TuneableCapturer)
	adapt := time.NewTicker(5 * time.Second)
	defer adapt.Stop()
	var seq uint32
	for {
		select {
		case <-h.stop:
			return
		case <-h.Mux.closed:
			return
		case <-adapt.C:
			if tune != nil {
				tune.SetQuality(qualityForRTT(h.Mux.RTT()))
			}
			continue
		case <-tick.C:
		}
		var fr ScreenFrame
		if dc != nil {
			full, tiles, err := dc.CaptureDelta()
			if err != nil {
				return
			}
			if full == nil && len(tiles) == 0 {
				continue
			}
			seq++
			// Periodic keyframe: late joiners and drift recovery.
			if seq%100 == 1 && full == nil {
				img, _, err := h.Cap.Capture()
				if err != nil {
					return
				}
				fr = ScreenFrame{Seq: seq, Full: true, Image: img}
			} else if full != nil {
				fr = ScreenFrame{Seq: seq, Full: true, Image: full}
			} else {
				fr = ScreenFrame{Seq: seq, Tiles: tiles}
			}
		} else {
			img, changed, err := h.Cap.Capture()
			if err != nil {
				return
			}
			if !changed {
				continue
			}
			seq++
			fr = ScreenFrame{Seq: seq, Full: true, Image: img}
		}
		h.hmu.RLock()
		rec := h.rec
		h.hmu.RUnlock()
		if rec != nil {
			rec.OnFrame(fr)
		}
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
		h.workerMu.Lock()
		close(h.stop)
		h.workerMu.Unlock()
		h.Mux.Close()
		if closer, ok := h.Cap.(io.Closer); ok {
			_ = closer.Close()
		}
		h.wg.Wait()
	})
}
