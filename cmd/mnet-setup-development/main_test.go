//go:build mnet_enrollment_development && linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fintex.au/mnet/internal/daemon"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevelopmentBrowserLauncherCannotHoldAttemptOpen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	if err := runDevelopmentBrowser(ctx, "https://127.0.0.1:1/disposable", nil); err == nil {
		t.Fatal("stalled launcher succeeded")
	}
	if time.Since(started) > 4500*time.Millisecond {
		t.Fatal("launcher remained blocked until outer attempt timeout")
	}
	if ctx.Err() != nil {
		t.Fatal("launcher consumed the outer attempt deadline")
	}
}

func TestDevelopmentBrowserLauncherHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if err := runDevelopmentBrowser(ctx, "https://127.0.0.1:1/disposable", nil); err == nil {
		t.Fatal("cancelled launch succeeded")
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancelled launcher blocked")
	}
}

func TestDevelopmentBrowserLauncherPreservesPrivateOutputAndFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xdg-open"), []byte("#!/bin/sh\nprintf launcher-failed\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(dir, "private-launch.log")
	log, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	launchErr := runDevelopmentBrowser(context.Background(), "https://127.0.0.1:1/disposable", log)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if launchErr == nil {
		t.Fatal("failed launcher succeeded")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "launcher-failed" {
		t.Fatal("private launcher output was not preserved")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private launcher log permissions changed")
	}
}

func TestDevelopmentJoinedResultPrecedesOptionalDisplay(t *testing.T) {
	for _, mode := range []string{"write-failure", "open-failure", "preexisting-page"} {
		t.Run(mode, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			event := daemon.DevelopmentEnrollmentEvent{Stage: "joined-not-connected", Peers: 0, Result: &daemon.DevelopmentEnrollmentResult{NativeStateFinalized: true}}
			err := reportDevelopmentJoined(json.NewEncoder(&output), event, func() error {
				var got daemon.DevelopmentEnrollmentEvent
				if json.Unmarshal(output.Bytes(), &got) != nil || got.Stage != "joined-not-connected" || got.Result == nil || !got.Result.NativeStateFinalized {
					t.Fatal("display ran before authoritative result")
				}
				return errors.New(mode)
			}, &diagnostics)
			if err != nil || !strings.Contains(diagnostics.String(), "Joining saved.") || strings.Contains(diagnostics.String(), "Setup did not finish") {
				t.Fatal("optional display failed the saved result")
			}
		})
	}
}

type developmentBrokenOutput struct{}

func (developmentBrokenOutput) Write([]byte) (int, error) { return 0, errors.New("unavailable output") }
func TestDevelopmentJoinedOutputFailureStillNamesSavedResult(t *testing.T) {
	displayed := false
	err := reportDevelopmentJoined(json.NewEncoder(developmentBrokenOutput{}), daemon.DevelopmentEnrollmentEvent{Stage: "joined-not-connected"}, func() error { displayed = true; return nil }, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "joining saved") || displayed {
		t.Fatal("output failure lost saved outcome or attempted display")
	}
}
