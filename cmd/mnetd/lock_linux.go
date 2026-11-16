//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"fintex.au/mnet/internal/privatefs"
	"golang.org/x/sys/unix"
)

// The stable lock file is never removed: unlinking it could let another process
// lock a new inode while this process still owns the original. Closing the
// descriptor (including on a crash) releases ownership without stale-PID rules.
func lockDaemonState(dir string) (func(), error) {
	if err := privatefs.EnsureDir(dir); err != nil {
		return nil, err
	}
	fd, err := unix.Open(filepath.Join(dir, "daemon.lock"), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, fmt.Errorf("open private daemon ownership lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), "daemon ownership lock")
	ok := false
	defer func() {
		if !ok {
			file.Close()
		}
	}()
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return nil, fmt.Errorf("inspect daemon ownership lock: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
		return nil, fmt.Errorf("daemon ownership lock must be a regular file with one link")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("another daemon already owns this state directory")
		}
		return nil, fmt.Errorf("acquire daemon ownership lock: %w", err)
	}
	if err = file.Chmod(0600); err != nil {
		return nil, fmt.Errorf("protect daemon ownership lock: %w", err)
	}
	ok = true
	return func() { file.Close() }, nil
}
