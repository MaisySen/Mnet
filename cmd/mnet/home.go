package main

import (
	"context"
	"errors"
	"net/http"
)

// The home window routes local journeys only. It never enrolls, discovers trust,
// starts a pair or carries authorization from a prior child window.
type homeChoice int

const (
	homeClose homeChoice = iota
	homeNetwork
	homeDesktop
	homeLayout
)

func runHome(ctx context.Context, hc *http.Client, base, layoutPath string) error {
	if ctx == nil {
		return errors.New("home owner unavailable")
	}
	notice := "Choose a tool to begin."
	for ctx.Err() == nil {
		choice, err := chooseHome(ctx, notice, layoutPath != "")
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
		case homeLayout:
			if layoutPath == "" {
				return errors.New("home layout route unavailable")
			}
			var preview *deskLayoutPreview
			preview, err = readDeskLayoutPreviewFile(layoutPath)
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
		} else if err != nil {
			notice = "The tool could not finish. Choose it again to try a fresh attempt."
		} else {
			notice = "Tool closed. You can choose another tool."
		}
	}
	return nil
}
func homeArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) == 2 && args[0] == "--layout" && args[1] != "" {
		return args[1], nil
	}
	return "", errors.New("usage: mnet open [--layout FILE] (local engineering entry; no sign-in)")
}
