//go:build linux

package main

import (
	"context"
	"net/http"
	"testing"
)

func TestDeviceWindowRefusesAbsentOwnerBeforeDisplayOrAPI(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, canceled} {
		if err := runDeviceWindow(ctx, &http.Client{}, "http://invalid.example"); err == nil {
			t.Fatal("window created without live owner")
		}
	}
}
