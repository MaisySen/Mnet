//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"

	"fintex.au/mnet/internal/session"
)

// Backend choice on Linux: Wayland sessions capture with grim on
// wlroots compositors (Sway, Hyprland, Wayfire, River) or through
// the XDG ScreenCast portal everywhere else (GNOME above all), and
// inject with ydotool; X11 sessions use XGetImage and XTEST. Wayland
// is preferred when WAYLAND_DISPLAY is set, because X11 capture under
// XWayland only sees X11 windows, not the real desktop. grim stays
// the default where it works (no consent dialog, lower overhead);
// MNET_CAPTURE=portal forces the portal backend.

func onWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

func have(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

func newCapturer() (session.Capturer, error) {
	if onWayland() {
		forcePortal := os.Getenv("MNET_CAPTURE") == "portal"
		if !forcePortal && have("grim") {
			// wlroots compositor: direct screencopy, no dialog.
			return newWaylandCapturer()
		}
		c, err := newPortalCapturer()
		if err != nil {
			if forcePortal {
				return nil, fmt.Errorf("Wayland portal capture: %w", err)
			}
			return nil, fmt.Errorf("Wayland session without grim (install grim on wlroots compositors); portal capture failed too: %w", err)
		}
		return c, nil
	}
	return newX11Capturer()
}

func newInjector() (session.Injector, error) {
	mode := os.Getenv("MNET_TEXT_MODE")
	if onWayland() {
		if !have("ydotool") {
			return nil, fmt.Errorf("Wayland session but ydotool not installed; install ydotool or use an X11 session")
		}
		inj := newWaylandInjector()
		if w, ok := inj.(*waylandInjector); ok {
			w.textMode = resolveTextMode(mode, "wayland", have)
		}
		return inj, nil
	}
	inj, err := newX11Injector()
	if err != nil {
		return nil, err
	}
	if x, ok := inj.(*x11Host); ok {
		x.textMode = resolveTextMode(mode, "x11", have)
	}
	return inj, nil
}
