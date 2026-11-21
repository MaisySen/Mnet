//go:build !linux

package main

import (
	"context"
	"errors"
)

func runDeskHomeJourney(context.Context, string, string) error {
	return errors.New("arrangements require local Linux X11")
}
