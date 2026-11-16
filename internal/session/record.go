package session

// Session recording: the host can tap its own screen stream into an
// MJPEG file (concatenated JPEG frames, which ffmpeg and most players
// read directly with -f mjpeg). Keyframes are written as received;
// delta frames are composited onto the stored base and re-encoded, so
// the recording is a complete, standalone video of the session.

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"sync"
)

type Recorder struct {
	mu     sync.Mutex
	w      *os.File
	base   *image.RGBA
	frames int
}

// NewRecorder creates a new recording (0600: recordings are sensitive).
// Existing destinations, including final symlinks, are never overwritten.
func NewRecorder(path string) (*Recorder, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Recorder{w: f}, nil
}

// OnFrame records one screen frame. Decode failures skip the frame;
// a delta before the first keyframe is skipped, as in viewers.
func (r *Recorder) OnFrame(fr ScreenFrame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.w == nil {
		return
	}
	if fr.Full || len(fr.Tiles) == 0 {
		if img, err := decodeScreenJPEG(fr.Image); err == nil {
			r.base = imageToRGBA(img)
		} else {
			r.base = nil // A broken keyframe invalidates following deltas.
			return
		}
		if _, err := r.w.Write(fr.Image); err == nil {
			r.frames++
		}
		return
	}
	if r.base == nil {
		return
	}
	merged, err := CompositeTiles(r.base, fr.Tiles)
	if err != nil {
		r.base = nil // Do not composite later deltas on an older frame.
		return
	}
	r.base = merged
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, merged, &jpeg.Options{Quality: 80}); err != nil {
		return
	}
	if _, err := r.w.Write(buf.Bytes()); err == nil {
		r.frames++
	}
}

// Frames recorded so far.
func (r *Recorder) Frames() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.frames
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.base = nil
	if r.w == nil {
		return nil
	}
	err := r.w.Close()
	r.w = nil
	return err
}

// String reports the recording shape for logs and audit lines.
func (r *Recorder) String() string {
	return fmt.Sprintf("%d frames", r.Frames())
}
