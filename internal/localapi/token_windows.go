//go:build windows

package localapi

import (
	"errors"
	"io"
	"os"
)

// Unix mode bits are not a Windows DACL. Dedicated-state ACL protection remains
// with privatefs; bound the read and verify the opened object is still regular.
func readTokenFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("local API token must be a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("local API token unavailable")
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return nil, errors.New("local API token changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, 65))
	if err != nil || len(data) != 64 {
		return nil, errors.New("invalid local API token length")
	}
	return data, nil
}
