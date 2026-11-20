//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"fintex.au/mnet/internal/desk"
)

type deskReviewUI interface {
	Review(context.Context, string, string) error
}
type deskZenityReview struct {
	path    string
	display string
}

func newDeskReviewUI() (deskReviewUI, error) {
	if onWayland() || os.Getenv("DISPLAY") == "" {
		return nil, errors.New("desk review requires local X11 desktop")
	}
	path, err := exec.LookPath("zenity")
	if err != nil {
		return nil, errors.New("desk review requires Zenity; no automatic fallback")
	}
	return deskZenityReview{path: path}, nil
}
func deskReviewText(role string, binding desk.Binding, layout *desk.Compiled) (string, string, error) {
	if err := binding.Validate(layout); err != nil {
		return "", "", err
	}
	// UI presents only exact independently supplied digests, never peer labels,
	// quoted message content, markup, command strings or claimed identities.
	for _, key := range []string{binding.Primary, binding.Recipient} {
		if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
			return "", "", errors.New("desk review requires exact TLS public-key pins")
		}
	}
	arrangement, err := deskArrangementReview(binding, layout)
	if err != nil {
		return "", "", err
	}
	action := "Allow this computer to receive raw keyboard and mouse input?"
	if role == "primary" {
		action = "Arm one edge to send this computer's keyboard and mouse?"
	} else if role != "recipient" {
		return "", "", errors.New("unknown desk review role")
	}
	text := fmt.Sprintf("%s\n\nCompare both exact device keys using an independent trusted source.\nThis dialog does not verify workspace membership or a person's identity.\n\nPrimary key:\n%s\nRecipient key:\n%s\n\nOne-use run:\n%s\nApproved layout:\n%s\nKeyboard map:\n%s\nRecipient display: %s\n%s\n\nRaw keys and pointer only. No video, clipboard, files or recording.\nCtrl+Alt+Shift+F12 stops this live X11 process.\nAn unrelated active keyboard grab may block the shortcut.\nKilling the injector can leave recipient keys/buttons held.\n\nLocal engineering mode, not Multies sign-in.\nDecline if the keys or arrangement are not independently known.", action, binding.Primary, binding.Recipient, binding.Run, binding.Layout, binding.Keyboard, binding.Display, arrangement)
	return "Mnet Shared Desk: review " + role, text, nil
}
func (z deskZenityReview) Review(ctx context.Context, title, text string) error {
	command := exec.CommandContext(ctx, z.path, "--question", "--no-markup", "--default-cancel", "--width=720", "--title="+title, "--text="+text, "--ok-label=_Allow this run", "--cancel-label=Decline")
	if z.display != "" {
		command.Env = append(os.Environ(), "DISPLAY="+z.display)
	}
	command.WaitDelay = time.Second
	var result dialogResult
	command.Stdout = &result
	command.Stderr = io.Discard
	if err := command.Run(); err != nil || ctx.Err() != nil || result.text != "" {
		return errors.New("desk local review declined, closed or unavailable")
	}
	return nil
}

// reviewDeskRun requires the caller's independently accepted binding and current
// authority before and after a fresh local answer. No native factory runs here.
func reviewDeskRun(ctx context.Context, role string, binding desk.Binding, layout *desk.Compiled, live func() bool, ui deskReviewUI) error {
	if ctx == nil || ui == nil || live == nil || !live() {
		return errors.New("desk review prerequisites absent")
	}
	title, text, err := deskReviewText(role, binding, layout)
	if err != nil {
		return err
	}
	prompt, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-prompt.Done():
				return
			case <-tick.C:
				if !live() {
					cancel()
					return
				}
			}
		}
	}()
	err = ui.Review(prompt, title, text)
	close(done)
	<-joined
	if err != nil {
		return err
	}
	if prompt.Err() != nil || !live() {
		return errors.New("desk review answer no longer current")
	}
	return nil
}
func reviewedDeskApproval(ctx context.Context, binding desk.Binding, layout *desk.Compiled, live func() bool, ui deskReviewUI, create func(func() bool) (desk.NativeInput, error)) (*desk.Approval, error) {
	if err := reviewDeskRun(ctx, "recipient", binding, layout, live, ui); err != nil {
		return nil, err
	}
	return desk.NewApproval(binding, layout, func() bool { return ctx.Err() == nil && live() }, create)
}
func reviewedDeskEdge(ctx context.Context, board *desk.Switchboard, edge desk.Edge, binding desk.Binding, layout *desk.Compiled, keyboard string, live func() bool, ui deskReviewUI, create func(desk.Binding) (desk.NativeInput, error)) (*deskPrimaryCapture, error) {
	if board == nil || ui == nil {
		return nil, errors.New("desk review board or UI absent")
	}
	if edge != desk.Left && edge != desk.Right && edge != desk.Top && edge != desk.Bottom {
		return nil, errors.New("desk review edge invalid")
	}
	exactUI := deskEdgeReview{ui, edge}
	if err := reviewDeskRun(ctx, "primary", binding, layout, live, exactUI); err != nil {
		return nil, errors.Join(err, board.Stop())
	}
	return waitDeskEdge(ctx, board, edge, binding, keyboard, live, create)
}

type deskEdgeReview struct {
	ui   deskReviewUI
	edge desk.Edge
}

func (e deskEdgeReview) Review(ctx context.Context, title, text string) error {
	return e.ui.Review(ctx, title, "Selected source edge: "+string(e.edge)+"\n\n"+text)
}
