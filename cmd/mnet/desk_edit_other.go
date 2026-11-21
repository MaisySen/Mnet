//go:build !linux

package main

import (
	"context"
	"errors"
)

func showDeskLayoutEditor(context.Context, *deskLayoutEditor, string) (bool, error) {
	return false, errors.New("offline layout editor requires local Linux X11")
}

func readDeskLayoutEditorFile(string) (*deskLayoutEditor, error) {
	return nil, errors.New("offline layout editor requires local Linux X11")
}
