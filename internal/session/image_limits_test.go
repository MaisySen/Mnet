package session

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"image/jpeg"
	"testing"
)

func oversizedJPEGHeader(t *testing.T) []byte {
	t.Helper()
	data := append([]byte(nil), jpegBytes(t, solid(8, 8, color.RGBA{A: 255}))...)
	// Rewrite the baseline SOF dimensions without creating a huge pixel buffer.
	i := bytes.Index(data, []byte{0xff, 0xc0})
	if i < 0 {
		t.Fatal("fixture SOF missing")
	}
	binary.BigEndian.PutUint16(data[i+5:i+7], 65535)
	binary.BigEndian.PutUint16(data[i+7:i+9], 65535)
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != 65535 || config.Height != 65535 {
		t.Fatalf("bad header fixture: %+v %v", config, err)
	}
	return data
}

func TestScreenJPEGPolicyBeforePixelDecode(t *testing.T) {
	for _, test := range []struct {
		w, h    int
		allowed bool
	}{{3840, 2160, true}, {4096, 4096, true}, {8192, 2048, true}, {8193, 1, false}, {8192, 2049, false}, {7680, 4320, false}, {0, 1, false}, {1, -1, false}, {int(^uint(0) >> 1), 2, false}} {
		if got := screenDimensionsAllowed(test.w, test.h); got != test.allowed {
			t.Fatalf("%dx%d allowed=%v", test.w, test.h, got)
		}
	}
	data := oversizedJPEGHeader(t)
	if _, err := screenJPEGConfig(data); err == nil {
		t.Fatal("oversized header accepted")
	}
	if _, err := decodeScreenJPEG(data); err == nil {
		t.Fatal("oversized JPEG decoded")
	}
	v := &Viewer{base: solid(8, 8, color.RGBA{A: 255})}
	fr := ScreenFrame{Full: true, Image: data}
	if v.resolveFrame(&fr) || v.base != nil {
		t.Fatal("viewer kept oversized keyframe chain")
	}
	r, err := NewRecorder(t.TempDir() + "/record.mjpeg")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.base = solid(8, 8, color.RGBA{A: 255})
	r.OnFrame(fr)
	if r.Frames() != 0 || r.base != nil {
		t.Fatal("recorder kept oversized keyframe chain")
	}
}
