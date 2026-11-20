//go:build !windows

package localapi

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Open and inspect the same descriptor. NONBLOCK keeps a substituted FIFO from
// hanging the CLI; NOFOLLOW rejects a substituted final-component link. Parent
// directories and the local account must still be trusted by the caller.
func readTokenFile(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("local API token unavailable")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		return nil, errors.New("local API token metadata unavailable")
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("local API token must be an owned private regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 65))
	if err != nil || len(data) != 64 {
		return nil, errors.New("invalid local API token length")
	}
	return data, nil
}
