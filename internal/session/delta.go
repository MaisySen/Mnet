package session

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
)

// Dirty-rect deltas: instead of a full JPEG every change, the host
// can send only the 64x64 tiles that differ from the previous frame.
// The viewer composites tiles onto the last full frame and always
// hands its callback a complete JPEG, so apps never see the delta
// machinery. Full frames and delta frames mix freely (the protocol
// carries both from day one).

const tileSize = 64

// Tile is one changed region's JPEG bytes.
type Tile struct {
	Rect  Rect   `json:"rect"`
	Image []byte `json:"image"`
}

// DiffTiles returns the tile rectangles whose pixels differ between
// prev and curr. A nil prev reports every tile (a full refresh).
func DiffTiles(prev, curr *image.RGBA, size int) []Rect {
	b := curr.Bounds()
	w, h := b.Dx(), b.Dy()
	var out []Rect
	for ty := 0; ty*size < h; ty++ {
		for tx := 0; tx*size < w; tx++ {
			r := image.Rect(tx*size, ty*size, min((tx+1)*size, w), min((ty+1)*size, h))
			if prev == nil || tileChanged(prev, curr, r) {
				out = append(out, Rect{X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy()})
			}
		}
	}
	return out
}

func tileChanged(a, b *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		ai := a.PixOffset(r.Min.X, y)
		bi := b.PixOffset(r.Min.X, y)
		na := ai + r.Dx()*4
		if !bytes.Equal(a.Pix[ai:na], b.Pix[bi:bi+r.Dx()*4]) {
			return true
		}
	}
	return false
}

// EncodeTiles cuts and JPEG-encodes each rect of img.
func EncodeTiles(img *image.RGBA, rects []Rect, quality int) ([]Tile, error) {
	out := make([]Tile, 0, len(rects))
	for _, r := range rects {
		sub := img.SubImage(image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H))
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, sub, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}
		out = append(out, Tile{Rect: r, Image: buf.Bytes()})
	}
	return out, nil
}

// CompositeTiles decodes tiles and paints them onto a copy of base.
func CompositeTiles(base *image.RGBA, tiles []Tile) (*image.RGBA, error) {
	if base == nil {
		return nil, fmt.Errorf("delta has no base")
	}
	bounds := base.Bounds()
	if !screenDimensionsAllowed(bounds.Dx(), bounds.Dy()) {
		return nil, fmt.Errorf("base dimensions exceed screen policy")
	}
	// Validate every rectangle/header before decoding or painting any tile.
	// Subtraction avoids overflow in attacker-controlled X+W / Y+H.
	for _, tile := range tiles {
		r := tile.Rect
		if r.W <= 0 || r.H <= 0 || r.X < bounds.Min.X || r.Y < bounds.Min.Y ||
			r.X >= bounds.Max.X || r.Y >= bounds.Max.Y ||
			r.W > bounds.Max.X-r.X || r.H > bounds.Max.Y-r.Y {
			return nil, fmt.Errorf("tile rectangle outside base")
		}
		config, err := screenJPEGConfig(tile.Image)
		if err != nil {
			return nil, err
		}
		if config.Width != r.W || config.Height != r.H {
			return nil, fmt.Errorf("tile JPEG dimensions do not match rectangle")
		}
	}
	out := image.NewRGBA(bounds)
	copy(out.Pix, base.Pix)
	for _, t := range tiles {
		img, err := jpeg.Decode(bytes.NewReader(t.Image))
		if err != nil {
			return nil, err
		}
		dst := image.Rect(t.Rect.X, t.Rect.Y, t.Rect.X+t.Rect.W, t.Rect.Y+t.Rect.H)
		for y := 0; y < t.Rect.H; y++ {
			for x := 0; x < t.Rect.W; x++ {
				out.Set(dst.Min.X+x, dst.Min.Y+y, img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y))
			}
		}
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
