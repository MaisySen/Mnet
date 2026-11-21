//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ensureDeskStore creates the private store directory on demand, only when
// the person chose an action that saves. Browsing never creates it.
func ensureDeskStore(dir string) error {
	if err := unix.Mkdir(dir, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
		return errStoreUnavailable
	}
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errStoreUnavailable
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return errStoreUnavailable
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		return errStoreUnavailable
	}
	return nil
}

// deskStoreOutPath names the next create-only destination inside an ensured
// private store. The save path still refuses an existing destination.
func deskStoreOutPath(dir string, entries []deskStoreEntry, extra ...string) (string, error) {
	if err := ensureDeskStore(dir); err != nil {
		return "", err
	}
	name := nextDeskStoreName(entries, extra...)
	if filepath.Base(name) != name {
		return "", errStoreUnavailable
	}
	return filepath.Join(dir, name), nil
}
