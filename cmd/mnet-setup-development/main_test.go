//go:build mnet_enrollment_development && linux

package main

import (
	"context"
	"os"
	"path/filepath"
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
