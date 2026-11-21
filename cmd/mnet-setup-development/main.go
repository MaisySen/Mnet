//go:build mnet_enrollment_development

// mnet-setup-development is an explicitly built disposable local setup entry.
// It is absent from normal builds/installers and does not register a service.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"fintex.au/mnet/internal/daemon"
)

func main() {
	root := flag.String("disposable-root", "", "existing private disposable root with explicit marker, policy and limits")
	open := flag.Bool("open-browser", false, "open local joining consent in the system browser")
	flag.Parse()
	if flag.NArg() != 0 || *root == "" {
		fmt.Fprintln(os.Stderr, "an explicit disposable setup root is required")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	encoder := json.NewEncoder(os.Stdout)
	ownedAttempt, saved, declined := false, false, false
	writeStatus := func(joined bool, peers int) error {
		title, body := "Setup did not finish", "Setup stopped without a confirmed completion result. A partial joining result may have been saved. The original local attempt is retained. Do not assume this computer is connected or start a replacement attempt."
		if declined {
			title = "Computer not approved"
			body = "You declined this joining request. This setup did not submit a device proof or save a joining credential. No tunnel, screen, keyboard or mouse access was started."
		}
		if joined {
			title = "Joining saved. Not connected."
			body = fmt.Sprintf("Mnet verified this computer and saved its joining result. The current map checked during setup contained %d live peers. This is a historical setup result, not a live connection indicator. No tunnel, screen, keyboard or mouse access was started.", peers)
		}
		file, err := os.OpenFile(filepath.Join(*root, "setup-result.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		page := template.Must(template.New("result").Parse(`<!doctype html><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><title>Mnet development setup result</title><style>body{font:17px system-ui;background:#f4f5f7;color:#202124;margin:0;padding:64px 24px}main{max-width:620px;margin:auto;background:white;border:1px solid #ddd;border-radius:18px;padding:32px}p{line-height:1.6}small{color:#596171}</style><main><small>DISPOSABLE DEVELOPMENT SETUP</small><h1>{{.Title}}</h1><p>{{.Body}}</p><p>You can close this tab.</p></main>`))
		err = page.Execute(file, struct{ Title, Body string }{title, body})
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if *open && runtime.GOOS == "linux" {
			return runDevelopmentBrowser(context.Background(), filepath.Join(*root, "setup-result.html"), nil)
		}
		return nil
	}
	err := daemon.RunDisposableEnrollment(ctx, *root, func(event daemon.DevelopmentEnrollmentEvent) error {
		if event.Stage == "waiting-for-browser" {
			ownedAttempt = true
		}
		if event.Stage == "declined" {
			declined = true
		}
		if event.Stage == "joined-not-connected" {
			saved = true
			if err := writeStatus(true, event.Peers); err != nil {
				return err
			}
		}
		if err := encoder.Encode(event); err != nil {
			return err
		}
		if event.BrowserURL != "" && *open {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("system browser opening is implemented only for Linux development")
			}
			// Browsers may outlive their launcher. Never hand a browser the
			// setup process output pipe, which would prevent its owner joining.
			log, err := os.OpenFile(filepath.Join(*root, "browser-launch.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("private browser launch log unavailable")
			}
			err = runDevelopmentBrowser(ctx, event.BrowserURL, log)
			log.Close()
			if err != nil {
				return fmt.Errorf("system browser could not open; original attempt retained")
			}
		}
		return nil
	})
	if err != nil {
		// Only a configured disposable root with an owned native attempt may
		// receive a historical failure page. Never create in a refused root.
		if ownedAttempt && !saved {
			_ = writeStatus(false, 0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Browser launch must return before an owned attempt can observe its deadline.
func runDevelopmentBrowser(ctx context.Context, target string, log *os.File) error {
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, "xdg-open", target)
	if log != nil {
		cmd.Stdout = log
		cmd.Stderr = log
	}
	return cmd.Run()
}
