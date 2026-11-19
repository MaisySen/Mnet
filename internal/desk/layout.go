// Package desk contains Shared Desk geometry, exact one-use pair transport and
// input ownership. It does not supply enrollment, local approval UI or workspace
// authority. Geometry alone never activates native input.
package desk

import (
	"errors"
	"fmt"
	"strings"
)

// Display's arrangement bounds use one common integer layout coordinate space.
// Pixel sizes are the native target coordinate space, not video-frame sizes.
// Rectangles are half-open. A device can own several separately named displays.
type Display struct {
	ID          string `json:"id"`
	Device      string `json:"device"`
	X           int64  `json:"x"`
	Y           int64  `json:"y"`
	Width       int64  `json:"width"`
	Height      int64  `json:"height"`
	PixelWidth  int64  `json:"pixel_width"`
	PixelHeight int64  `json:"pixel_height"`
}
type Layout struct {
	Version  int       `json:"version"`
	Displays []Display `json:"displays"`
}
type Edge string

const (
	Left   Edge = "left"
	Right  Edge = "right"
	Top    Edge = "top"
	Bottom Edge = "bottom"
)

type Link struct {
	From  string `json:"from"`
	Edge  Edge   `json:"edge"`
	To    string `json:"to"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
}

// Compiled owns a copy of validated geometry. There is no active device or grant.
type Compiled struct {
	displays []Display
	links    []Link
}
type Candidate struct {
	Display string `json:"display"`
	Device  string `json:"device"`
	X       int64  `json:"x"`
	Y       int64  `json:"y"`
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c)) {
			return false
		}
	}
	return true
}
func Compile(layout Layout) (*Compiled, error) {
	if layout.Version != 1 {
		return nil, errors.New("unsupported offline layout version")
	}
	if len(layout.Displays) < 2 || len(layout.Displays) > 32 {
		return nil, errors.New("layout requires2to32displays")
	}
	c := &Compiled{displays: append([]Display(nil), layout.Displays...)}
	seen := make(map[string]bool)
	for _, d := range c.displays {
		if !validID(d.ID) || !validID(d.Device) || seen[d.ID] {
			return nil, errors.New("display/device IDs must be bounded unique display identifiers")
		}
		seen[d.ID] = true
		if d.X < -1000000 || d.X > 1000000 || d.Y < -1000000 || d.Y > 1000000 || d.Width < 1 || d.Width > 1000000 || d.Height < 1 || d.Height > 1000000 || d.PixelWidth < 1 || d.PixelWidth > 65535 || d.PixelHeight < 1 || d.PixelHeight > 65535 {
			return nil, errors.New("display geometry exceeds offline layout bounds")
		}
	}
	for i, a := range c.displays {
		for j, b := range c.displays {
			if i == j {
				continue
			}
			ox, ex := max(a.X, b.X), min(a.X+a.Width, b.X+b.Width)
			oy, ey := max(a.Y, b.Y), min(a.Y+a.Height, b.Y+b.Height)
			if ox < ex && oy < ey {
				return nil, errors.New("display arrangement overlaps")
			}
			add := func(edge Edge, start, end int64) {
				if start < end {
					c.links = append(c.links, Link{a.ID, edge, b.ID, start, end})
				}
			}
			if a.X == b.X+b.Width {
				add(Left, oy, ey)
			}
			if a.X+a.Width == b.X {
				add(Right, oy, ey)
			}
			if a.Y == b.Y+b.Height {
				add(Top, ox, ex)
			}
			if a.Y+a.Height == b.Y {
				add(Bottom, ox, ex)
			}
		}
	}
	// No source-edge point can name two destinations. Do not pick whichever
	// appeared first in a file, including within one multi-display device.
	for i, a := range c.links {
		for _, b := range c.links[i+1:] {
			if a.From == b.From && a.Edge == b.Edge && max(a.Start, b.Start) < min(a.End, b.End) {
				return nil, errors.New("ambiguous shared edge")
			}
		}
	}
	reached := map[string]bool{c.displays[0].ID: true}
	for changed := true; changed; {
		changed = false
		for _, link := range c.links {
			if reached[link.From] && !reached[link.To] {
				reached[link.To] = true
				changed = true
			}
		}
	}
	if len(reached) != len(c.displays) {
		return nil, errors.New("layout contains disconnected or corner-only displays")
	}
	return c, nil
}
func (c *Compiled) Links() []Link {
	if c == nil {
		return nil
	}
	return append([]Link(nil), c.links...)
}

// MapEdge maps a source native-pixel tangent position onto an explicitly named
// edge. It is a geometry candidate, not evidence an edge was physically crossed.
// Gaps/dead corners return no candidate. It does not replay overshoot or input.
func (c *Compiled) MapEdge(source string, edge Edge, along int64) (Candidate, bool, error) {
	if c == nil {
		return Candidate{}, false, errors.New("layout unavailable")
	}
	var a Display
	found := false
	for _, d := range c.displays {
		if d.ID == source {
			a = d
			found = true
			break
		}
	}
	if !found {
		return Candidate{}, false, errors.New("unknown source display")
	}
	vertical := edge == Left || edge == Right
	if !vertical && edge != Top && edge != Bottom {
		return Candidate{}, false, errors.New("invalid edge")
	}
	origin, size, pixels := a.X, a.Width, a.PixelWidth
	if vertical {
		origin, size, pixels = a.Y, a.Height, a.PixelHeight
	}
	if along < 0 || along >= pixels {
		return Candidate{}, false, errors.New("source edge pixel outside display")
	}
	numerator := origin*pixels + along*size
	for _, link := range c.links {
		if link.From != source || link.Edge != edge || numerator < link.Start*pixels || numerator >= link.End*pixels {
			continue
		}
		for _, b := range c.displays {
			if b.ID != link.To {
				continue
			}
			result := Candidate{Display: b.ID, Device: b.Device}
			if vertical {
				result.Y = (numerator - b.Y*pixels) * b.PixelHeight / (pixels * b.Height)
				if edge == Left {
					result.X = b.PixelWidth - 1
				}
			} else {
				result.X = (numerator - b.X*pixels) * b.PixelWidth / (pixels * b.Width)
				if edge == Top {
					result.Y = b.PixelHeight - 1
				}
			}
			if result.X < 0 || result.Y < 0 || result.X >= b.PixelWidth || result.Y >= b.PixelHeight {
				return Candidate{}, false, fmt.Errorf("mapped pixel outside validated destination")
			}
			return result, true, nil
		}
	}
	return Candidate{}, false, nil
}
