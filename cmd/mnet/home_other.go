//go:build !linux

package main

import (
	"context"
	"errors"
)

func chooseHome(context.Context, string, bool) (homeChoice, error) {
	return homeClose, errors.New("local engineering home currently requires Linux X11")
}
