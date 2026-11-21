//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package control

import (
	"fmt"
	"os"
	"syscall"

	"fintex.au/mnet/internal/privatefs"
	"golang.org/x/sys/unix"
)

func singleLink(_ string, info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 {
		return fmt.Errorf("store file must have exactly one link")
	}
	return nil
}

func lockStoreFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, fmt.Errorf("open store ownership: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	fail := func(err error) (*os.File, error) { file.Close(); return nil, err }
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("store lock must be regular"))
	}
	if err := singleLink(path, info); err != nil {
		return fail(err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fail(fmt.Errorf("store already owned or lock unavailable: %w", err))
	}
	if err := privatefs.ProtectFile(path); err != nil {
		return fail(err)
	}
	return file, nil
}

func syncStoreDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		dir.Close()
		return err
	}
	return dir.Close()
}

func renameStoreFile(from, to string) error { return os.Rename(from, to) }
