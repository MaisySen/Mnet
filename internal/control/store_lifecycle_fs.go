package control

import (
	"fintex.au/mnet/internal/stateowner"
	"os"
)

// Keep the actual Store API/seams unchanged while sharing platform mechanics.
func singleLink(path string, info os.FileInfo) error { return stateowner.SingleLink(path, info) }
func syncStoreDir(path string) error                 { return stateowner.SyncDir(path) }
func renameStoreFile(from, to string) error          { return stateowner.Rename(from, to) }
