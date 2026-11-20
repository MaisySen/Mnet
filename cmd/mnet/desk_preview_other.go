//go:build !linux

package main

import (
	"context"
	"errors"
)

func showDeskLayoutPreview(context.Context, *deskLayoutPreview) error {
	return errors.New("offline layout preview requires local Linux X11")
}

func readDeskLayoutPreviewFile(string) (*deskLayoutPreview, error) {
	return nil, errors.New("offline layout preview requires local Linux X11")
}
