//go:build !linux

package main

import (
	"context"
	"errors"
	"net/http"
)

func runDeviceWindow(context.Context, *http.Client, string) error {
	return errors.New("read-only native device window currently requires Linux X11; use mnet status on this platform")
}
