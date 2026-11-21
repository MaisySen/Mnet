//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package stateowner

import (
	"fmt"
	"os"
)

func SingleLink(_ string, _ os.FileInfo) error {
	return fmt.Errorf("owned control Store is unsupported on this platform")
}
func lockFile(string) (*os.File, error) {
	return nil, fmt.Errorf("owned control Store is unsupported on this platform")
}
func SyncDir(string) error {
	return fmt.Errorf("owned control Store is unsupported on this platform")
}
func Rename(string, string) error {
	return fmt.Errorf("owned control Store is unsupported on this platform")
}
