//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package control

import (
	"fmt"
	"os"
)

func singleLink(_ string, _ os.FileInfo) error {
	return fmt.Errorf("owned control Store is unsupported on this platform")
}
func lockStoreFile(string) (*os.File, error) {
	return nil, fmt.Errorf("owned control Store is unsupported on this platform")
}
func syncStoreDir(string) error {
	return fmt.Errorf("owned control Store is unsupported on this platform")
}
func renameStoreFile(string, string) error {
	return fmt.Errorf("owned control Store is unsupported on this platform")
}
