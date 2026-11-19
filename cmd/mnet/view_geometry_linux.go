//go:build linux

package main

import "image"

const viewerHeader = 72

type viewerViewport struct {
	one  bool
	x, y int
}
type viewerImageRegion struct{ source, target image.Rectangle }

// One-to-one means one decoded received pixel per local pixel. It does not
// undo host capture downscaling or JPEG compression. Empty/small viewports
// never invent source pixels; frame/window changes clamp the owned pan.
func (v *viewerViewport) region(sw, sh, dw, dh int) viewerImageRegion {
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return viewerImageRegion{}
	}
	if !v.one {
		w, h := fitView(sw, sh, dw, dh)
		return viewerImageRegion{image.Rect(0, 0, sw, sh), image.Rect((dw-w)/2, (dh-h)/2, (dw-w)/2+w, (dh-h)/2+h)}
	}
	w, h := min(sw, dw), min(sh, dh)
	v.x = max(0, min(v.x, sw-w))
	v.y = max(0, min(v.y, sh-h))
	return viewerImageRegion{image.Rect(v.x, v.y, v.x+w, v.y+h), image.Rect((dw-w)/2, (dh-h)/2, (dw-w)/2+w, (dh-h)/2+h)}
}
func (v *viewerViewport) key(sym uint32, sw, sh, dw, dh int) bool {
	switch sym {
	case '1':
		v.one = true
	case 'f', 'F':
		v.one = false
	case 0xff50:
		v.x = 0
		v.y = 0 // Home
	case 0xff57:
		v.x = sw
		v.y = sh // End, clamped below
	case 0xff51:
		if !v.one {
			return false
		}
		v.x -= 64
	case 0xff53:
		if !v.one {
			return false
		}
		v.x += 64
	case 0xff52:
		if !v.one {
			return false
		}
		v.y -= 64
	case 0xff54:
		if !v.one {
			return false
		}
		v.y += 64
	default:
		return false
	}
	v.region(sw, sh, dw, dh)
	return true
}
