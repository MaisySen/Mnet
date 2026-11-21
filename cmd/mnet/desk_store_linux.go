//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

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

// errDeskDraftChanged marks a same-name replacement or edit detected between
// arming and confirming a delete; the delete cancels and removes nothing.
var errDeskDraftChanged = errors.New("arrangement draft identity changed")

// deskDraftID pins one store draft's identity across the delete confirmation
// so a same-name replacement is never removed in its place.
type deskDraftID struct {
	dev, ino uint64
	size     int64
	mtime    unix.Timespec
}

// statDeskDraft returns the pinned identity of one app-owned draft. It
// refuses a swapped or symlinked store directory, anything but a regular
// file, and any draft aliased by a second hard link.
func statDeskDraft(dir, name string) (deskDraftID, error) {
	var zero deskDraftID
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return zero, errors.New("invalid arrangement draft name")
	}
	var dst unix.Stat_t
	if err := unix.Lstat(dir, &dst); err != nil {
		return zero, err
	}
	if dst.Mode&unix.S_IFMT != unix.S_IFDIR {
		return zero, errors.New("arrangement store is not a real directory")
	}
	var st unix.Stat_t
	if err := unix.Lstat(filepath.Join(dir, name), &st); err != nil {
		return zero, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return zero, errors.New("refusing to remove anything but a draft file")
	}
	if st.Nlink != 1 {
		return zero, errors.New("refusing an aliased draft")
	}
	return deskDraftID{dev: uint64(st.Dev), ino: st.Ino, size: st.Size, mtime: st.Mtim}, nil
}

// deleteDeskStoreEntry removes the pinned draft only while its identity
// still matches exactly; any change cancels with errDeskDraftChanged.
// The residual race between the final stat and unlink cannot be closed
// without fd-relative unlink; it is bounded by the private 0700 store under
// the explicit state directory, which only a same-uid process can reach, and
// it can never cross outside the store.
func deleteDeskStoreEntry(dir, name string, want deskDraftID) error {
	got, err := statDeskDraft(dir, name)
	if err != nil {
		return err
	}
	if got != want {
		return errDeskDraftChanged
	}
	return os.Remove(filepath.Join(dir, name))
}
