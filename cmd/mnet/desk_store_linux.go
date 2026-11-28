//go:build linux

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// openDeskStoreDir pins the real store directory for fd-relative operations:
// a swapped or symlinked path can never redirect a source read, a
// destination create or a cleanup outside the directory opened here.
func openDeskStoreDir(dir string) (int, error) {
	dfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, errors.New("arrangement store is not a real directory")
	}
	var st unix.Stat_t
	if err := unix.Fstat(dfd, &st); err != nil {
		unix.Close(dfd)
		return -1, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		unix.Close(dfd)
		return -1, errors.New("arrangement store must be a private owner-only directory")
	}
	return dfd, nil
}

// validDeskDraftName accepts only plain store-local base names.
func validDeskDraftName(name string) bool {
	return name != "" && name == filepath.Base(name) && !strings.HasPrefix(name, ".")
}

// errDeskDraftChanged marks a same-name replacement or edit detected between
// arming and confirming a delete; the delete cancels and removes nothing.
var errDeskDraftChanged = errors.New("arrangement draft identity changed")

// deskDraftID records changes to a draft, including edits with a restored mtime.
type deskDraftID struct {
	dev, ino uint64
	size     int64
	mtime    unix.Timespec
	ctime    unix.Timespec
}

// Keep the draft open across confirmation so its inode cannot be recycled.
type deskDeletePin struct {
	id   deskDraftID
	file *os.File
}

func (p *deskDeletePin) close() {
	if p != nil && p.file != nil {
		p.file.Close()
	}
}

func deskDraftIdentity(st unix.Stat_t) (deskDraftID, error) {
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return deskDraftID{}, errors.New("refusing to remove anything but a draft file")
	}
	if st.Nlink != 1 {
		return deskDraftID{}, errors.New("refusing an aliased draft")
	}
	return deskDraftID{dev: uint64(st.Dev), ino: st.Ino, size: st.Size, mtime: st.Mtim, ctime: st.Ctim}, nil
}

func pinDeskDraftDelete(dir, name string) (*deskDeletePin, error) {
	if !validDeskDraftName(name) {
		return nil, errors.New("invalid arrangement draft name")
	}
	dfd, err := openDeskStoreDir(dir)
	if err != nil {
		return nil, err
	}
	defer unix.Close(dfd)
	fd, err := unix.Openat(dfd, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		unix.Close(fd)
		return nil, err
	}
	id, err := deskDraftIdentity(st)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &deskDeletePin{id: id, file: os.NewFile(uintptr(fd), name)}, nil
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
	return deskDraftIdentity(st)
}

// deleteDeskStoreEntry removes the pinned draft only while its identity
// still matches exactly; any change cancels with errDeskDraftChanged.
func deleteDeskStoreEntry(dir, name string, want *deskDeletePin) error {
	if !validDeskDraftName(name) {
		return errors.New("invalid arrangement draft name")
	}
	dfd, err := openDeskStoreDir(dir)
	if err != nil {
		return err
	}
	defer unix.Close(dfd)
	var st unix.Stat_t
	if err := unix.Fstatat(dfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	got, err := deskDraftIdentity(st)
	if err != nil {
		return err
	}
	if want == nil || want.file == nil || want.file.Fd() == ^uintptr(0) || got != want.id {
		return errDeskDraftChanged
	}
	// The unlink is relative to the pinned private directory; the remaining
	// boundary is the file-identity race between stat and unlink, same-uid
	// only, unable to cross outside the store.
	err = unix.Unlinkat(dfd, name, 0)
	runtime.KeepAlive(want.file)
	return err
}

// duplicateDeskStoreEntry copies one pinned store draft into a new
// exclusively-created owner-only file. The source must keep its pinned
// identity through the read; the destination must not exist and is never
// followed or replaced. Nothing outside the store is read or written.
func duplicateDeskStoreEntry(dir, name, out string, want deskDraftID) error {
	if out == "" || out != filepath.Base(out) || out == name || strings.HasPrefix(out, ".") {
		return errors.New("invalid arrangement draft name")
	}
	got, err := statDeskDraft(dir, name)
	if err != nil {
		return err
	}
	if got != want {
		return errDeskDraftChanged
	}
	dfd, err := openDeskStoreDir(dir)
	if err != nil {
		return err
	}
	defer unix.Close(dfd)
	// Every step below is relative to the pinned private directory: a
	// swapped or symlinked store path cannot redirect the source read, the
	// destination create or the cleanup.
	fd, err := unix.Openat(dfd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	// The os.File owns the descriptor; a manual unix.Close would leave its
	// finalizer to close a reused descriptor (an X connection) later.
	src := os.NewFile(uintptr(fd), name)
	data, err := io.ReadAll(io.LimitReader(src, (1<<20)+1))
	// Fstat on the open descriptor pins the identity of the very file read,
	// so a same-name replacement during the read cancels instead of copying.
	var again unix.Stat_t
	ferr := unix.Fstat(fd, &again)
	src.Close()
	if err != nil || len(data) > 1<<20 || ferr != nil {
		return errors.New("arrangement draft unreadable")
	}
	if (deskDraftID{dev: uint64(again.Dev), ino: again.Ino, size: again.Size, mtime: again.Mtim, ctime: again.Ctim}) != want {
		return errDeskDraftChanged
	}
	wfd, err := unix.Openat(dfd, out, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return errors.New("arrangement draft destination already exists")
	}
	file := os.NewFile(uintptr(wfd), out)
	_, werr := file.Write(data)
	cerr := file.Close()
	if werr != nil || cerr != nil {
		unix.Unlinkat(dfd, out, 0)
		return errors.New("arrangement draft copy incomplete")
	}
	return nil
}
