//go:build !linux && !darwin

package main

// Linux and macOS currently have the state-directory flock guard. Other platforms retain
// their existing local API binding behavior; no cross-platform singleton claim.
func lockDaemonState(dir string) (func(), error) { return func() {}, nil }
