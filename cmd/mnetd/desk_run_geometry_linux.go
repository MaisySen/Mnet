//go:build linux

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// The reviewed recipient geometry must stay exact while waiting as well as
// during injection. No input owner or network listener is created by this guard.
type deskRunGeometryGuard struct {
	ctx     context.Context
	cancel  context.CancelFunc
	conn    *xgb.Conn
	done    chan struct{}
	changed atomic.Bool
}

func newDeskRunGeometryGuard(parent context.Context, spec desk.RunSpec) (*deskRunGeometryGuard, error) {
	if parent == nil || parent.Err() != nil || spec.Role != "recipient" {
		return nil, errors.New("desk geometry owner unavailable")
	}
	var width, height int64
	for _, d := range spec.Layout.Displays {
		if d.ID == spec.Binding.Display && d.Device == spec.Binding.Recipient {
			width, height = d.PixelWidth, d.PixelHeight
		}
	}
	if width < 64 || height < 64 {
		return nil, errors.New("desk reviewed recipient geometry unavailable")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return nil, errors.New("desk recipient display unavailable")
	}
	setup := xproto.Setup(c)
	screen := setup.DefaultScreen(c)
	fail := func() (*deskRunGeometryGuard, error) {
		c.Close()
		return nil, errors.New("desk recipient geometry differs from reviewed layout")
	}
	if len(setup.Roots) != 1 {
		return fail()
	}
	if err = xproto.ChangeWindowAttributesChecked(c, screen.Root, xproto.CwEventMask, []uint32{xproto.EventMaskStructureNotify}).Check(); err != nil {
		return fail()
	}
	exact := func() bool {
		geometry, err := xproto.GetGeometry(c, xproto.Drawable(screen.Root)).Reply()
		return err == nil && int64(geometry.Width) == width && int64(geometry.Height) == height
	}
	if !exact() {
		return fail()
	}
	ctx, cancel := context.WithCancel(parent)
	g := &deskRunGeometryGuard{ctx: ctx, cancel: cancel, conn: c, done: make(chan struct{})}
	type result struct {
		event xgb.Event
		err   error
	}
	events := make(chan result, 1)
	eventDone := make(chan struct{})
	go func() {
		defer close(eventDone)
		for {
			event, err := c.WaitForEvent()
			select {
			case events <- result{event, err}:
			case <-ctx.Done():
				return
			}
			if err != nil || event == nil {
				return
			}
		}
	}()
	go func() {
		defer close(g.done)
		defer func() { cancel(); c.Close(); <-eventDone }()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			lost := false
			select {
			case <-ctx.Done():
				return
			case r := <-events:
				if ctx.Err() != nil {
					return
				}
				if r.err != nil || r.event == nil {
					lost = true
				} else if e, ok := r.event.(xproto.ConfigureNotifyEvent); ok {
					lost = e.Window == screen.Root && (int64(e.Width) != width || int64(e.Height) != height)
				}
			case <-tick.C:
				if ctx.Err() != nil {
					return
				}
				lost = !exact()
			}
			if ctx.Err() != nil {
				return
			}
			if lost {
				g.changed.Store(true)
				return
			}
		}
	}()
	return g, nil
}
func (g *deskRunGeometryGuard) Close() error {
	g.cancel()
	g.conn.Close()
	<-g.done
	if g.changed.Load() {
		return errors.New("desk reviewed recipient geometry changed or unavailable")
	}
	return nil
}
