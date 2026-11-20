package main

import (
	"errors"
	"fintex.au/mnet/internal/session"
)

type desktopPointerAction struct {
	Kind  string
	Input session.InputEvent
	Grant session.ControlStatus
}
type desktopPointerView interface {
	desktopView
	PointerStatus(session.ControlStatus) error
	PointerActions() []desktopPointerAction
}

func dispatchDesktopPointer(display desktopPointerView, control *session.ViewerControl, viewer *session.Viewer) error {
	for _, action := range display.PointerActions() {
		if control == nil || viewer == nil {
			continue
		}
		current := control.Snapshot()
		switch action.Kind {
		case "request":
			if action.Grant.Session != current.Session || !current.PointerV1 {
				continue
			}
			if current.State == "available" || current.State == "declined" || current.State == "revoked" {
				if err := control.RequestPointer(); err != nil {
					return err
				}
			}
		case "release":
			if current.State == "pending" || current.State == "allowed" || current.State == "paused" {
				if err := control.Withdraw(); err != nil {
					return err
				}
			}
		case "input":
			// Never relabel a queued event with a successor/resumed grant. The bind is
			// atomic with the final status comparison inside ViewerControl.
			if current != action.Grant || current.State != "allowed" {
				continue
			}
			if err := viewer.SendInputFor(action.Grant, action.Input); err != nil {
				if control.Snapshot() != action.Grant {
					continue
				}
				return err
			}
		default:
			return errors.New("unknown native pointer action")
		}
	}
	if control != nil {
		return display.PointerStatus(control.Snapshot())
	}
	return nil
}
