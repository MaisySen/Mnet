package main

import (
	"fintex.au/mnet/internal/privatefs"
	"fintex.au/mnet/internal/stateowner"
	"path/filepath"
)

// Compatibility lock probe for existing command tests. Production ownership is
// exclusively daemon.New/Close; runDaemon must not stack this lease around New.
func lockDaemonState(dir string) (func(), error) {
	if err := privatefs.EnsureDir(dir); err != nil {
		return nil, err
	}
	lease, _, err := stateowner.Acquire(filepath.Join(dir, "daemon"))
	if err != nil {
		return nil, err
	}
	return func() { lease.Close() }, nil
}
