//go:build linux

package main

import (
	"errors"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const attendedInjectorOwnerAtom = "_MNET_ATTENDED_INJECTOR_OWNER_V1"

type attendedInjectorOwner struct {
	conn   *xgb.Conn
	window xproto.Window
	done   chan struct{}
	once   sync.Once
	err    error
}

// This claim lives in the surviving injector process, not its coordinator.
// It arbitrates only cooperating injectors on this X server.
func claimAttendedInjectorOwner(stop func()) (*attendedInjectorOwner, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, errors.New("local injector owner unavailable")
	}
	success := false
	defer func() {
		if !success {
			c.Close()
		}
	}()
	atom, err := xproto.InternAtom(c, false, uint16(len(attendedInjectorOwnerAtom)), attendedInjectorOwnerAtom).Reply()
	if err != nil {
		return nil, errors.New("local injector owner unavailable")
	}
	window, err := xproto.NewWindowId(c)
	if err != nil {
		return nil, err
	}
	root := xproto.Setup(c).DefaultScreen(c).Root
	if err = xproto.CreateWindowChecked(c, 0, window, root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0, 0, nil).Check(); err != nil {
		return nil, err
	}
	if err = xproto.GrabServerChecked(c).Check(); err != nil {
		return nil, err
	}
	current, readErr := xproto.GetSelectionOwner(c, atom.Atom).Reply()
	if readErr != nil || current.Owner != 0 {
		xproto.UngrabServer(c)
		c.Sync()
		_ = xproto.DestroyWindowChecked(c, window).Check()
		return nil, errors.New("another attended injector owns this desktop")
	}
	err = xproto.SetSelectionOwnerChecked(c, window, atom.Atom, xproto.TimeCurrentTime).Check()
	xproto.UngrabServer(c)
	c.Sync()
	if err != nil {
		return nil, err
	}
	o := &attendedInjectorOwner{conn: c, window: window, done: make(chan struct{})}
	go func() {
		defer close(o.done)
		for {
			event, err := c.WaitForEvent()
			if err != nil || event == nil {
				stop()
				return
			}
			switch event.(type) {
			case xproto.SelectionClearEvent:
				stop()
				return
			}
		}
	}()
	success = true
	return o, nil
}
func (o *attendedInjectorOwner) Close() error {
	o.once.Do(func() { o.err = xproto.DestroyWindowChecked(o.conn, o.window).Check(); o.conn.Close() })
	<-o.done
	return o.err
}
