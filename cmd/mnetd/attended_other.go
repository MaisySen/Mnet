//go:build !linux

package main

import (
	"errors"

	"fintex.au/mnet/internal/session"
)

func newAttendedUI() (attendedUI, error) {
	return nil, errors.New("attended desktop hosting is currently Linux X11 only")
}

func newAttendedInjector() (session.Injector, error) {
	return nil, errors.New("attended input unavailable")
}
