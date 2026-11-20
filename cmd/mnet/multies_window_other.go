//go:build !linux

package main

import (
	"context"
	"errors"
	"net/http"
)

func runMultiesWindow(context.Context, *http.Client, string, string) error {
	return errors.New("experimental Multies window currently requires Linux X11; no browser or account action was started")
}
