// Package portal: turning the negotiated PipeWire stream into raw
// frames.
//
// Speaking the PipeWire protocol natively (SPA POD serialisation,
// buffer negotiation, memfd data buffers) is a project of its own;
// every maintained pure-Go attempt is incomplete. The industrial
// answer is the one OBS and friends use: let the platform's own
// multimedia stack decode. We hand the portal's connected socket fd
// and the stream node to a gst-launch helper that decodes and scales
// to a known raw format on stdout; the capturer then reads
// fixed-size frames off the pipe. gst-launch-1.0 with pipewiresrc is
// part of the standard PipeWire/GStreamer install on every GNOME
// system this backend targets.

package portal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Pump reads raw BGRA frames of a fixed geometry from a PipeWire
// stream.
type Pump struct {
	cmd    *exec.Cmd
	out    io.Reader
	frame  int // bytes per frame: w*h*4
	Width  int
	Height int
}

// PumpCommand builds the gst-launch argv for a stream. The socket fd
// is handed to the child as its fd 3 (first ExtraFiles slot), which
// is what pipewiresrc's fd property points at.
func PumpCommand(node uint32, fd, width, height int) []string {
	return []string{
		"gst-launch-1.0", "-q",
		"pipewiresrc", fmt.Sprintf("fd=%d", 3), fmt.Sprintf("path=%d", node), "do-timestamp=true",
		"!", "videoconvert",
		"!", "videoscale",
		"!", fmt.Sprintf("video/x-raw,format=BGRA,width=%d,height=%d", width, height),
		"!", "fdsink", "fd=1",
	}
}

// StartPump launches the helper for a stream. pwfd stays owned by the
// caller; the child gets its own duplicate via ExtraFiles.
func StartPump(s *Stream, pwfd *os.File, width, height int) (*Pump, error) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		return nil, fmt.Errorf("gst-launch-1.0 not installed (package gstreamer1.0-tools): %w", err)
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("stream geometry unknown (the compositor reported no size)")
	}
	cmd := exec.Command(PumpCommand(s.NodeID, 3, width, height)[0], PumpCommand(s.NodeID, 3, width, height)[1:]...)
	cmd.ExtraFiles = []*os.File{pwfd}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gst helper: %w", err)
	}
	return &Pump{cmd: cmd, out: out, frame: width * height * 4, Width: width, Height: height}, nil
}

// NewRawPump builds a Pump over an arbitrary reader producing raw
// BGRA frames of the given geometry. Used by tests (in this package
// and by the mnetd capturer) and by the selftest rig.
func NewRawPump(r io.Reader, width, height int) *Pump {
	return &Pump{out: r, frame: width * height * 4, Width: width, Height: height}
}

// ReadFrame blocks until one full raw frame is available or the
// helper dies. Callers own the returned buffer only until the next
// ReadFrame when they pass reuse.
func (p *Pump) ReadFrame(reuse []byte) ([]byte, error) {
	if len(reuse) != p.frame {
		reuse = make([]byte, p.frame)
	}
	if _, err := io.ReadFull(p.out, reuse); err != nil {
		return nil, fmt.Errorf("gst helper: %w", err)
	}
	return reuse, nil
}

// Close stops the helper.
func (p *Pump) Close() error {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return p.cmd.Wait()
}
