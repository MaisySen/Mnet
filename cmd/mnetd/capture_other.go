//go:build !linux

package main

import (
	"errors"

	"fintex.au/mnet/internal/session"
)

func newCapturer() (session.Capturer, error) {
	return nil, errors.New("no screen capture backend on this platform yet")
}

func newInjector() (session.Injector, error) {
	return nil, errors.New("no input injection backend on this platform yet")
}
