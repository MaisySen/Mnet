//go:build !linux

package main

import "errors"

func newAttendedSafety() (attendedSafety, error) {
	return nil, errors.New("attended safety is currently Linux X11 only")
}
