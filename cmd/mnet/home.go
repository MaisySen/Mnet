package main

import (
	"context"
	"errors"
	"net/http"

	"fintex.au/mnet/internal/multies"
)

// The home window routes local journeys only. It never enrolls, discovers trust,
// starts a pair or carries authorization from a prior child window.
type homeChoice int

const (
	homeClose homeChoice = iota
	homeNetwork
	homeDesktop
	homeLayout
	homeMulties
)

type homeConfig struct{ LayoutPath, MultiesOrigin string }

func runHome(ctx context.Context, hc *http.Client, base string, config homeConfig) error {
	if ctx == nil {
		return errors.New("home owner unavailable")
	}
	notice := "Choose a tool to begin."
	for ctx.Err() == nil {
		choice, err := chooseHome(ctx, notice, config.LayoutPath != "", config.MultiesOrigin != "")
		if err != nil {
			return err
		}
		if choice == homeClose {
			return nil
		}
		switch choice {
		case homeNetwork:
			err = runDeviceWindow(ctx, hc, base)
		case homeDesktop:
			err = browseDesktopOnce(ctx, hc, base)
		case homeMulties:
			if config.MultiesOrigin == "" {
				return errors.New("Multies origin not configured")
			}
			err = runMultiesWindow(ctx, hc, base, config.MultiesOrigin)
		case homeLayout:
			if config.LayoutPath == "" {
				return errors.New("home layout route unavailable")
			}
			var preview *deskLayoutPreview
			preview, err = readDeskLayoutPreviewFile(config.LayoutPath)
			if err == nil {
				err = showDeskLayoutPreview(ctx, preview)
			}
		default:
			return errors.New("unknown home route")
		}
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, errDesktopFirstScreen) {
			notice = "No screen arrived. The session ended. Choose Remote Desktop to try again."
		} else if choice == homeMulties {
			notice = "Approval review closed. Network setup is still not connected."
			if err != nil {
				notice = "Approval review was unavailable. Network setup is still not connected."
			}
		} else if err != nil {
			notice = "The tool could not finish. Choose it again to try a fresh attempt."
		} else {
			notice = "Tool closed. You can choose another tool."
		}
	}
	return nil
}

// Configuration comes only from the explicit local invocation. Never read an
// origin from a peer, browser return, device label or prior sign-in result.
func homeArgs(args []string) (homeConfig, error) {
	var c homeConfig
	bad := errors.New("usage: mnet open [--layout FILE] [--multies-server TRUSTED_ORIGIN] (approvals only, no enrollment)")
	for len(args) > 0 {
		if len(args) < 2 || args[1] == "" {
			return homeConfig{}, bad
		}
		switch args[0] {
		case "--layout":
			if c.LayoutPath != "" {
				return homeConfig{}, bad
			}
			c.LayoutPath = args[1]
		case "--multies-server":
			if c.MultiesOrigin != "" || len(args[1]) > 75 {
				return homeConfig{}, bad
			}
			for _, r := range args[1] {
				if r < 33 || r > 126 {
					return homeConfig{}, bad
				}
			}
			if _, err := multies.NewBridgeClient(args[1]); err != nil {
				return homeConfig{}, bad
			}
			c.MultiesOrigin = args[1]
		default:
			return homeConfig{}, bad
		}
		args = args[2:]
	}
	return c, nil
}
