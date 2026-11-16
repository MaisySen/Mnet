//go:build !linux

package main

// Linux currently has the state-directory flock guard. Other platforms retain
// their existing local API binding behavior; no cross-platform singleton claim.
func lockDaemonState(dir string) (func(), error) { return func() {}, nil }
