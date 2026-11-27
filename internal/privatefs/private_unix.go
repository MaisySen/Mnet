//go:build !windows

package privatefs

import "os"

func protect(path string, dir bool) error {
	if dir {
		return os.Chmod(path, 0700)
	}
	return os.Chmod(path, 0600)
}

// CreateFile exclusively creates a private, writable file.
func CreateFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}

// CreateTemp exclusively creates a private temporary file in dir with prefix.
func CreateTemp(dir, prefix string) (*os.File, error) {
	return os.CreateTemp(dir, prefix+"*")
}

// MkdirTemp exclusively creates a private temporary directory in dir with prefix.
func MkdirTemp(dir, prefix string) (string, error) {
	return os.MkdirTemp(dir, prefix+"*")
}
