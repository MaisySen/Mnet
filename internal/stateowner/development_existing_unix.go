//go:build mnet_enrollment_development && (linux || darwin || freebsd || openbsd || netbsd || dragonfly)

package stateowner

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// AcquireExistingDevelopment owns an EXISTING stable lock without creating,
// chmod, syncing or writing any file. Only explicitly tagged development reads
// use it. Missing/aliased/insecure files fail instead of being repaired.
func AcquireExistingDevelopment(path string) (*Lease, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("existing ownership path required")
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return nil, fmt.Errorf("existing canonical parent required")
	}
	pi, err := os.Stat(parent)
	if err != nil || !pi.IsDir() || pi.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("existing private parent required")
	}
	lockPath := path + ".lock"
	fd, err := unix.Open(lockPath, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), lockPath)
	fail := func(e error) (*Lease, error) { file.Close(); return nil, e }
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fail(fmt.Errorf("existing private lock required"))
	}
	if err := SingleLink(lockPath, info); err != nil {
		return fail(err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fail(err)
	}
	lease := &Lease{file: file, path: lockPath, parent: parent, parentInfo: pi}
	if err := lease.Check(); err != nil {
		return fail(err)
	}
	return lease, nil
}
