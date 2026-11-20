//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"fintex.au/mnet/internal/session"
)

type zenityUI struct{ path string }

func newAttendedUI() (attendedUI, error) {
	if onWayland() || os.Getenv("DISPLAY") == "" {
		return nil, errors.New("attended host currently requires a local X11 desktop")
	}
	path, err := exec.LookPath("zenity")
	if err != nil {
		return nil, errors.New("attended host requires Zenity; no automatic fallback")
	}
	return zenityUI{path}, nil
}
func attendedDialogArgs(p attendedPrompt) ([]string, error) {
	args := []string{"--question", "--no-markup", "--default-cancel", "--width=480"}
	switch p.Kind {
	case "view":
		args = append(args, "--title=Mnet: allow viewing?", "--text="+p.Peer+" wants to view this desktop.\n\nAllow viewing only? Control needs another local approval.\nFiles and recording are unavailable.\n\nEngineering mesh session, not Multies sign-in.", "--ok-label=Allow viewing", "--cancel-label=Decline")
	case "control":
		if p.Scope == session.ControlScopePointer {
			args = append(args, "--title=Mnet: allow pointer only?", "--text="+p.Peer+" requests pointer-only control.\n\nAllow pointer movement, atomic clicks and single wheel steps?\nKeyboard input and button holds/dragging are refused by this host.\nViewing alone grants no input. Pause/revoke/end remain available.", "--ok-label=Allow pointer", "--cancel-label=View only")
			break
		}
		if p.Scope != "" {
			return nil, errors.New("unsupported local control scope")
		}
		args = append(args, "--title=Mnet: allow control?", "--text="+p.Peer+" requests keyboard and pointer control.\n\nAllow only if you trust this peer.\nViewing alone does not grant control.\nUse the session window to pause, revoke or end.", "--ok-label=Allow control", "--cancel-label=View only")
	case "active":
		args = []string{"--info", "--no-markup", "--width=480"}
		args = append(args, "--title=Mnet: desktop sharing", "--text="+p.Peer+" can view this desktop.\n\nControl needs a separate local approval.\nPause stops control before asking to resume.\nRevoke returns to view only.\nClosing this window ends sharing.\nLocal stop shortcut: Ctrl+Alt+Shift+F12.\nRequires this app and the X server to be responsive.\n\nFiles and recording are unavailable.", "--ok-label=End session", "--extra-button=Pause control", "--extra-button=Revoke control")
	default:
		return nil, errors.New("unknown local prompt")
	}
	return args, nil
}

type dialogResult struct{ text string }

func (r *dialogResult) Write(p []byte) (int, error) {
	if len(r.text)+len(p) > 128 {
		return 0, errors.New("unexpected dialog output")
	}
	r.text += string(p)
	return len(p), nil
}
func (z zenityUI) Ask(ctx context.Context, p attendedPrompt) (string, error) {
	args, err := attendedDialogArgs(p)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, z.path, args...)
	cmd.WaitDelay = time.Second
	var result dialogResult
	cmd.Stdout = &result
	cmd.Stderr = io.Discard
	err = cmd.Run()
	// Zenity returns status 1 for extra buttons, the same as Cancel. Only the
	// exact fixed active-window label plus that exit status can select an action.
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if p.Kind != "active" || !ok || exit.ExitCode() != 1 || (strings.TrimSpace(result.text) != "Pause control" && strings.TrimSpace(result.text) != "Revoke control") {
			return "", errors.New("local dialog closed or unavailable")
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	text := strings.TrimSpace(result.text)
	if p.Kind != "active" {
		if text != "" {
			return "", errors.New("unexpected dialog result")
		}
		return "allow", nil
	}
	switch text {
	case "Pause control":
		return "pause", nil
	case "Revoke control":
		return "revoke", nil
	case "":
		return "end", nil
	default:
		return "", errors.New("unexpected dialog result")
	}
}

// Attended control never changes clipboard contents through an environment
// override. The dedicated local X11 injector owns its grant-lifetime ledger.
func newAttendedInjector() (session.Injector, error) { return startAttendedProcessInjector() }
