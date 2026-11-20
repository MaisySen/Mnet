//go:build !linux

package main

import (
	"context"
	"errors"
	"net/http"
)

func chooseDesktopPeer(context.Context, *http.Client, string) (*desktopPeerChoice, error) {
	return nil, errors.New("native peer chooser currently requires local Linux X11")
}
