//go:build !linux

package main

import "errors"

func newDesktopView(string) (desktopView, error) {
	return nil, errors.New("native desktop viewing currently requires a local Linux X11 display")
}
