//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/desk"
)

type deskReviewFixture struct {
	ask func(context.Context, string, string) error
}

func (f deskReviewFixture) Review(ctx context.Context, title, text string) error {
	return f.ask(ctx, title, text)
}
func deskReviewBindingFixture(t *testing.T) (desk.Binding, *desk.Compiled) {
	t.Helper()
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	layout, err := desk.Compile(desk.Layout{Version: 1, Displays: []desk.Display{{ID: "source", Device: a, Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768}, {ID: "recipient", Device: b, X: 100, Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768}}})
	if err != nil {
		t.Fatal(err)
	}
	return desk.Binding{Run: strings.Repeat("c", 64), Primary: a, Recipient: b, Layout: layout.Fingerprint(), Keyboard: strings.Repeat("d", 64), Display: "recipient"}, layout
}
func TestDeskReviewExactEvidenceAndDecline(t *testing.T) {
	binding, layout := deskReviewBindingFixture(t)
	ui := deskReviewFixture{func(ctx context.Context, title, text string) error {
		for _, want := range []string{binding.Primary, binding.Recipient, binding.Run, binding.Layout, binding.Keyboard, "No video, clipboard, files or recording", "not Multies sign-in", "leave recipient keys/buttons held"} {
			if !strings.Contains(text, want) {
				t.Fatal("review omitted exact scope", want)
			}
		}
		return errors.New("declined")
	}}
	if _, err := reviewedDeskApproval(context.Background(), binding, layout, func() bool { return true }, ui, func(func() bool) (desk.NativeInput, error) { t.Fatal("decline created native input"); return nil, nil }); err == nil {
		t.Fatal("declined review approved")
	}
}
func TestDeskReviewAuthorityLossCancelsPromptAndLateAnswer(t *testing.T) {
	binding, layout := deskReviewBindingFixture(t)
	var live atomic.Bool
	live.Store(true)
	entered := make(chan struct{})
	ui := deskReviewFixture{func(ctx context.Context, _, _ string) error { close(entered); <-ctx.Done(); return nil }}
	done := make(chan error, 1)
	go func() { done <- reviewDeskRun(context.Background(), "recipient", binding, layout, live.Load, ui) }()
	<-entered
	live.Store(false)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("late review answer accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("retired authority left review open")
	}
}

func TestDeskX11ReviewDenialAndAuthorityLoss(t *testing.T) {
	artifacts := os.Getenv("MNET_DESK_ARTIFACTS")
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" || artifacts == "" {
		t.Skip("requires native dialog driver")
	}
	binding, layout := deskReviewBindingFixture(t)
	actual, err := newDeskReviewUI()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"decline", "withdraw"} {
		t.Run(mode, func(t *testing.T) {
			var live atomic.Bool
			live.Store(true)
			ui := deskReviewFixture{func(ctx context.Context, title, text string) error { return actual.Review(ctx, title+" / "+mode, text) }}
			done := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				_, err := reviewedDeskApproval(ctx, binding, layout, live.Load, ui, func(func() bool) (desk.NativeInput, error) {
					t.Error("denied native review created input")
					return nil, nil
				})
				done <- err
			}()
			if mode == "withdraw" {
				until := time.Now().Add(10 * time.Second)
				for {
					if _, err = os.Stat(filepath.Join(artifacts, "review-withdraw.visible")); err == nil {
						break
					}
					if time.Now().After(until) {
						t.Fatal("withdraw review was not visible")
					}
					time.Sleep(time.Millisecond)
				}
				live.Store(false)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("native denied review approved")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("native denied review did not retire")
			}
		})
	}
}
