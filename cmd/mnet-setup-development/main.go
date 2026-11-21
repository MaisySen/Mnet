//go:build mnet_enrollment_development

// mnet-setup-development is an explicitly built disposable local setup entry.
// It is absent from normal builds/installers and does not register a service.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

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
	err := daemon.RunDisposableEnrollment(ctx, *root, func(event daemon.DevelopmentEnrollmentEvent) error {
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
			cmd := exec.CommandContext(ctx, "xdg-open", event.BrowserURL)
			cmd.Stdout = log
			cmd.Stderr = log
			err = cmd.Run()
			log.Close()
			if err != nil {
				return fmt.Errorf("system browser could not open; original attempt retained")
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
