//go:build !linux

package main

import (
	"context"
	"errors"
	"io"
)

func runDeskFile(context.Context, string, io.Writer) error {
	return errors.New("explicit Shared Desk run requires Linux X11; no fallback")
}

func inspectDeskLocal(string, io.Writer) error {
	return errors.New("local Shared Desk inspection requires Linux X11; no fallback")
}
