//go:build darwin

package main

import (
	"fmt"
	"os"
)

func isServiceSession() bool       { return false }
func runAsService(o options) error { return fmt.Errorf("use the generated launchd plist on macOS") }

// Generate only. No launchctl invocation, host file writes or enrollment.
func runServiceAction(o options) error {
	if o.serviceAction != "print-plist" {
		return fmt.Errorf("macOS supports -service print-plist only; installation is separate")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	plist, err := darwinServicePlist(exe, o)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, plist)
	return err
}
