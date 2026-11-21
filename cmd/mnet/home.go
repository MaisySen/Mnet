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

// Fixed home notices. The order is the machine-readable note slug in the
// window title; fixtures read outcomes from it instead of pixels.
const (
	homeNoticeStart        = "Choose a tool to begin."
	homeNoticeNoScreen     = "No screen arrived. The session ended. Choose Remote Desktop to try again."
	homeNoticeReviewClosed = "Approval review closed. Network setup is still not connected."
	homeNoticeReviewFailed = "Approval review was unavailable. Network setup is still not connected."
	homeNoticeFailed       = "The tool could not finish. Choose it again to try a fresh attempt."
	homeNoticeClosed       = "Tool closed. You can choose another tool."
	homeNoticeViewed       = "Viewing ended. Nothing remains connected. Choose a tool to continue."
	homeNoticeDeclined     = "The host did not allow viewing. Nothing was shared. Choose Remote Desktop to ask again."
	homeNoticeChanged      = "The device changed before connecting. Nothing was shared. Choose Remote Desktop to review it again."
	homeNoticeHostEnded    = "The host or the connection ended the viewing session. Nothing remains connected."
)

var homeNotices = []string{homeNoticeStart, homeNoticeNoScreen, homeNoticeReviewClosed, homeNoticeReviewFailed, homeNoticeFailed, homeNoticeClosed, homeNoticeViewed, homeNoticeDeclined, homeNoticeChanged, homeNoticeHostEnded}

func homeNoticeIndex(notice string) int {
	for i, n := range homeNotices {
		if n == notice {
			return i
		}
	}
	return -1
}

// desktopHomeNotice names the specific Remote Desktop outcome. A clean return
// still distinguishes a session that showed the host's screen from a chooser
// closed before any request; failures never claim a session existed.
func desktopHomeNotice(viewed bool, err error) string {
	switch {
	case errors.Is(err, errDesktopFirstScreen):
		return homeNoticeNoScreen
	case errors.Is(err, errDesktopHostEnded):
		return homeNoticeHostEnded
	case desktopViewRefused(err):
		return homeNoticeDeclined
	case errors.Is(err, errDesktopPeerChanged), errors.Is(err, errCLISessionIdentity):
		return homeNoticeChanged
	case err != nil:
		return homeNoticeFailed
	case viewed:
		return homeNoticeViewed
	default:
		return homeNoticeClosed
	}
}

func runHome(ctx context.Context, hc *http.Client, base string, config homeConfig) error {
	if ctx == nil {
		return errors.New("home owner unavailable")
	}
	notice := homeNoticeStart
	for ctx.Err() == nil {
		choice, err := chooseHome(ctx, notice, config.LayoutPath != "", config.MultiesOrigin != "")
		if err != nil {
			return err
		}
		if choice == homeClose {
			return nil
		}
		viewed := false
		switch choice {
		case homeNetwork:
			err = runDeviceWindow(ctx, hc, base)
		case homeDesktop:
			viewed, err = browseDesktopOnce(ctx, hc, base)
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
		switch {
		case choice == homeDesktop:
			notice = desktopHomeNotice(viewed, err)
		case choice == homeMulties:
			notice = homeNoticeReviewClosed
			if err != nil {
				notice = homeNoticeReviewFailed
			}
		case err != nil:
			notice = homeNoticeFailed
		default:
			notice = homeNoticeClosed
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
