//go:build linux

package main

import (
	"context"
	"errors"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// waitDeskEdge arms exactly one independently approved edge. It cannot discover,
// approve, persist or reconnect a pair. Source input stays local until an
// interior-to-edge arrival with no held keys/buttons. The caller must join
// capture/recipient cleanup on every error and may not reuse this run.
func waitDeskEdge(ctx context.Context, board *desk.Switchboard, edge desk.Edge, binding desk.Binding, keyboard string, live func() bool, create func(desk.Binding) (desk.NativeInput, error)) (captureResult *deskPrimaryCapture, result error) {
	if ctx == nil || board == nil || live == nil || !live() || create == nil {
		return nil, errors.New("desk edge arm prerequisites unavailable")
	}
	if edge != desk.Left && edge != desk.Right && edge != desk.Top && edge != desk.Bottom {
		return nil, errors.New("desk arm edge invalid")
	}
	activated := false
	defer func() {
		if !activated {
			result = errors.Join(result, board.Stop())
		}
	}()
	local, err := board.PrimaryDisplay()
	if err != nil {
		return nil, err
	}
	if current, remote := board.Destination(); remote || current.ID != local.ID {
		return nil, errors.New("desk source not active locally")
	}
	safety, err := newDeskSafety(keyboard)
	if err != nil {
		return nil, err
	}
	closed := false
	defer func() {
		if !closed {
			result = errors.Join(result, safety.Close())
		}
	}()
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)
	if local.PixelWidth < 64 || local.PixelHeight < 64 || local.PixelWidth > 32767 || local.PixelHeight > 32767 {
		return nil, errors.New("desk armed source geometry unsupported")
	}
	if len(setup.Roots) != 1 || int64(screen.WidthInPixels) != local.PixelWidth || int64(screen.HeightInPixels) != local.PixelHeight {
		return nil, errors.New("desk armed layout differs from source display")
	}
	exact, err := deskKeyboardFingerprint(conn)
	if err != nil || exact != keyboard {
		return nil, errors.New("desk armed keyboard differs")
	}
	current := func() bool { return ctx.Err() == nil && live() }
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	interior := false
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-board.Stopped():
			return nil, errors.New("desk arm stopped")
		case <-safety.Stopped():
			return nil, errors.New("desk arm local stop")
		case <-tick.C:
		}
		if !live() {
			return nil, errors.New("desk arm authority retired")
		}
		geometry, err := xproto.GetGeometry(conn, xproto.Drawable(screen.Root)).Reply()
		if err != nil || int64(geometry.Width) != local.PixelWidth || int64(geometry.Height) != local.PixelHeight {
			return nil, errors.New("desk armed display changed")
		}
		pointer, err := xproto.QueryPointer(conn, screen.Root).Reply()
		if err != nil || !pointer.SameScreen {
			return nil, errors.New("desk arm pointer unavailable")
		}
		// Require a fresh visit to the interior, never activate because enable
		// happened while the pointer was already resting on the selected edge.
		atEdge := false
		var along int64
		switch edge {
		case desk.Left:
			atEdge = pointer.RootX == 0
			along = int64(pointer.RootY)
		case desk.Right:
			atEdge = int64(pointer.RootX) == local.PixelWidth-1
			along = int64(pointer.RootY)
		case desk.Top:
			atEdge = pointer.RootY == 0
			along = int64(pointer.RootX)
		case desk.Bottom:
			atEdge = int64(pointer.RootY) == local.PixelHeight-1
			along = int64(pointer.RootX)
		}
		if !atEdge {
			interior = true
			continue
		}
		if !interior {
			continue
		}
		interior = false
		if pointer.Mask&0x1fff != 0 {
			continue
		}
		keys, err := xproto.QueryKeymap(conn).Reply()
		if err != nil {
			return nil, err
		}
		held := false
		for _, mask := range keys.Keys {
			held = held || mask != 0
		}
		if held {
			continue
		}
		// Release this arm's cooperating owner before the new capture claims its
		// own. Capture rechecks preheld state, map and actual source dimensions.
		if err = safety.Close(); err != nil {
			return nil, err
		}
		closed = true
		if ctx.Err() != nil || !live() {
			return nil, errors.New("desk arm retired before entry")
		}
		if err = board.Cross(edge, along, binding, create); err != nil {
			return nil, err
		}
		if ctx.Err() != nil || !live() {
			return nil, errors.Join(errors.New("desk arm retired during entry"), board.Stop())
		}
		capture, err := newDeskPrimaryCapture(board, keyboard, current)
		if err != nil {
			return nil, errors.Join(err, board.Stop())
		}
		activated = true
		return capture, nil
	}
}
