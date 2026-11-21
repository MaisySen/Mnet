package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"fintex.au/mnet/internal/desk"
)

// An offline arrangement editor. It edits a validated in-memory copy of one
// layout document and can save only to a new explicitly named file. It never
// pairs devices, starts a run, reads a daemon or grants any input authority.
// Labels stay local file data, never verified people or devices.
type deskLayoutEditor struct {
	layout   desk.Layout
	original desk.Layout
	selected int
	edge     desk.Edge
	undo     []desk.Layout
}

const deskEditUndoLimit = 64

func cloneDeskLayout(l desk.Layout) desk.Layout {
	l.Displays = append([]desk.Display(nil), l.Displays...)
	return l
}

func equalDeskLayout(a, b desk.Layout) bool {
	if a.Version != b.Version || len(a.Displays) != len(b.Displays) {
		return false
	}
	for i := range a.Displays {
		if a.Displays[i] != b.Displays[i] {
			return false
		}
	}
	return true
}

// The layout must already satisfy the strict decoder and Compile, exactly as
// the read-only inspector requires. Editing starts from valid geometry.
func newDeskLayoutEditor(layout desk.Layout) (*deskLayoutEditor, error) {
	if _, err := desk.Compile(layout); err != nil {
		return nil, err
	}
	return &deskLayoutEditor{
		layout:   cloneDeskLayout(layout),
		original: cloneDeskLayout(layout),
		edge:     desk.Right,
	}, nil
}

func readDeskLayoutEditor(r io.Reader) (*deskLayoutEditor, error) {
	layout, err := desk.Decode(r)
	if err != nil {
		return nil, err
	}
	return newDeskLayoutEditor(layout)
}

func (e *deskLayoutEditor) display() desk.Display { return e.layout.Displays[e.selected] }

func (e *deskLayoutEditor) dirty() bool { return !equalDeskLayout(e.layout, e.original) }

// validate is the exact Compile gate a saved file must pass. Save stays
// disabled while the in-memory arrangement is invalid.
func (e *deskLayoutEditor) validate() error {
	_, err := desk.Compile(e.layout)
	return err
}

func (e *deskLayoutEditor) selectDisplay(step int) {
	count := len(e.layout.Displays)
	e.selected = (e.selected + step%count + count) % count
}

func (e *deskLayoutEditor) selectEdge(edge desk.Edge) error {
	if edge != desk.Left && edge != desk.Right && edge != desk.Top && edge != desk.Bottom {
		return errors.New("invalid diagram edge")
	}
	e.edge = edge
	return nil
}

// pushUndo records the pre-change state once an op actually changes geometry.
func (e *deskLayoutEditor) pushUndo() {
	e.undo = append(e.undo, cloneDeskLayout(e.layout))
	if len(e.undo) > deskEditUndoLimit {
		copy(e.undo, e.undo[1:])
		e.undo = e.undo[:deskEditUndoLimit]
	}
}

func (e *deskLayoutEditor) undoEdit() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.layout = e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	if e.selected >= len(e.layout.Displays) {
		e.selected = len(e.layout.Displays) - 1
	}
	return true
}

func clampDesk(v, low, high int64) int64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

// moveDisplay shifts the selected display in layout units. Clamp, never wrap.
func (e *deskLayoutEditor) moveDisplay(dx, dy int64) bool {
	d := &e.layout.Displays[e.selected]
	nx := clampDesk(d.X+dx, -1000000, 1000000-d.Width)
	ny := clampDesk(d.Y+dy, -1000000, 1000000-d.Height)
	if nx == d.X && ny == d.Y {
		return false
	}
	e.pushUndo()
	d.X, d.Y = nx, ny
	return true
}

// resizeDisplay changes the selected display's layout-unit size.
func (e *deskLayoutEditor) resizeDisplay(dw, dh int64) bool {
	d := &e.layout.Displays[e.selected]
	nw := clampDesk(d.Width+dw, 1, 1000000)
	nh := clampDesk(d.Height+dh, 1, 1000000)
	if nw == d.Width && nh == d.Height {
		return false
	}
	e.pushUndo()
	d.Width, d.Height = nw, nh
	return true
}

// resizePixels changes the selected display's native pixel dimensions.
func (e *deskLayoutEditor) resizePixels(dpw, dph int64) bool {
	d := &e.layout.Displays[e.selected]
	npw := clampDesk(d.PixelWidth+dpw, 1, 65535)
	nph := clampDesk(d.PixelHeight+dph, 1, 65535)
	if npw == d.PixelWidth && nph == d.PixelHeight {
		return false
	}
	e.pushUndo()
	d.PixelWidth, d.PixelHeight = npw, nph
	return true
}

func deskEditFreeID(used func(string) bool, prefix string) string {
	for n := 1; ; n++ {
		id := fmt.Sprintf("%s-%d", prefix, n)
		if !used(id) {
			return id
		}
	}
}

// addDisplay attaches a same-sized copy of the selected display to the chosen
// edge, sharing that whole edge so the pair is linked by exact geometry. The
// new display becomes selected. A copy may overlap a third display; that is
// shown as an invalid arrangement for the person to fix, never silently nudged.
func (e *deskLayoutEditor) addDisplay() error {
	if len(e.layout.Displays) >= 32 {
		return errors.New("arrangement already has the maximum 32 displays")
	}
	d := e.display()
	n := d
	switch e.edge {
	case desk.Right:
		n.X = d.X + d.Width
	case desk.Left:
		n.X = d.X - d.Width
	case desk.Bottom:
		n.Y = d.Y + d.Height
	case desk.Top:
		n.Y = d.Y - d.Height
	}
	n.ID = deskEditFreeID(func(id string) bool {
		for _, other := range e.layout.Displays {
			if other.ID == id {
				return true
			}
		}
		return false
	}, "display")
	n.Device = deskEditFreeID(func(id string) bool {
		for _, other := range e.layout.Displays {
			if other.Device == id {
				return true
			}
		}
		return false
	}, "device")
	e.pushUndo()
	e.layout.Displays = append(e.layout.Displays, n)
	e.selected = len(e.layout.Displays) - 1
	return nil
}

// removeDisplay deletes the selected display. An arrangement needs at least
// two displays, so the last removable display refuses. Removing a link in the
// middle of a chain leaves a disconnected arrangement; that state is shown
// with its exact reason until the person repairs or undoes it.
func (e *deskLayoutEditor) removeDisplay() error {
	if len(e.layout.Displays) <= 2 {
		return errors.New("an arrangement keeps at least 2 displays")
	}
	e.pushUndo()
	i := e.selected
	e.layout.Displays = append(e.layout.Displays[:i], e.layout.Displays[i+1:]...)
	if e.selected >= len(e.layout.Displays) {
		e.selected = len(e.layout.Displays) - 1
	}
	return nil
}

// saveBytes is the only export path. It requires unsaved changes, a currently
// valid arrangement, and produces exactly one strict-decodable document. The
// caller owns safe placement of these bytes in a new file.
func (e *deskLayoutEditor) saveBytes() ([]byte, error) {
	if !e.dirty() {
		return nil, errors.New("no unsaved arrangement changes")
	}
	if err := e.validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(e.layout, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Diagram projection matches the read-only inspector: one aspect-preserving
// rational scale, rounding only for paint. Rounded pixels never edit geometry.
func (e *deskLayoutEditor) rectangles(width, height int64) ([]deskPreviewRect, error) {
	p := deskLayoutPreview{layout: e.layout, selected: e.selected, edge: e.edge}
	return p.rectangles(width, height)
}

// Direct mouse selection keeps the inspector's rule: one unambiguous display
// with a full 44px paint target. Selecting a rectangle only selects it.
func (e *deskLayoutEditor) hitDisplay(width, height, x, y int64) int {
	p := deskLayoutPreview{layout: e.layout, selected: e.selected, edge: e.edge}
	return p.hitDisplay(width, height, x, y)
}
