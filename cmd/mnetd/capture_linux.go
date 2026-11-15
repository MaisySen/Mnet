//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"

	"fintex.au/mnet/internal/session"
)

// Backend choice on Linux: Wayland sessions capture with grim and
// inject with ydotool (the portal-native D-Bus + PipeWire path is
// later work); X11 sessions use XGetImage and XTEST. Wayland is
// preferred when WAYLAND_DISPLAY is set, because X11 capture under
// XWayland only sees X11 windows, not the real desktop.

func onWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

func have(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

func newCapturer() (session.Capturer, error) {
	if onWayland() {
		if !have("grim") {
			return nil, fmt.Errorf("Wayland session but grim not installed; install grim (wlroots) or use an X11 session")
		}
		return newWaylandCapturer()
	}
	return newX11Capturer()
}

func newInjector() (session.Injector, error) {
	if onWayland() {
		if !have("ydotool") {
			return nil, fmt.Errorf("Wayland session but ydotool not installed; install ydotool or use an X11 session")
		}
		return newWaylandInjector(), nil
	}
	return newX11Injector()
}
