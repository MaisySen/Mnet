//go:build !windows && !linux && !darwin

package main

import "fmt"

// Service mode is Windows-only; these stubs keep dispatch portable.

func isServiceSession() bool { return false }

func runAsService(o options) error {
	return fmt.Errorf("service mode is only supported on Windows")
}

func runServiceAction(o options) error {
	return fmt.Errorf("-service is only supported on Windows")
}
