//go:build linux

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Static diagram files cannot start a blocking device/FIFO read. Inspect the
// opened descriptor, not a prior path stat. Explicit regular-file symlinks keep
// their existing behavior; the already-read snapshot never follows later edits.
func readDeskLayoutPreviewFile(path string) (*deskLayoutPreview, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("layout file unavailable")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("offline layout file must be regular")
	}
	return readDeskLayoutPreview(f)
}
