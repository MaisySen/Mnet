//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
)

// Per-link sysctl is an explicit exception to no global sysctl mutations. It
// writes one exact disable_ipv6 leaf in the resident owned namespace, never all
// or default. O_NOFOLLOW and dir-relative fd walk prevent symlink substitution.
// Missing IPv6 tree is NOT called disabled here: needs independent kernel
// address-family support proof; for now it blocks rather than guessing.
func disableOwnedIPv6(ns *namespaceCustody, c *ownedCreation, l Link, verifyLink func() error) error {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if ns == nil || verifyLink == nil {
		return ErrConflict
	}
	if e := ns.checkCurrent(); e != nil {
		return e
	}
	if e := verifyLink(); e != nil {
		return e
	}
	if !strings.HasPrefix(l.Name, "mno-") || strings.ContainsAny(l.Name, "/\x00") || len(l.Name) > 15 {
		return ErrConflict
	}
	if c == nil || c.handle.Link != l {
		return ErrConflict
	}
	if e := c.beforeEffect(c.handle, "config-intent"); e != nil {
		return e
	}
	fd, e := unix.Open("/proc/sys/net/ipv6/conf", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	linkFD, e := unix.Openat(fd, l.Name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return e
	}
	defer unix.Close(linkFD)
	leaf, e := unix.Openat(linkFD, "disable_ipv6", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(leaf), "owned-link-disable-ipv6")
	defer f.Close()
	if e = ns.checkCurrent(); e != nil {
		return e
	}
	if e = verifyLink(); e != nil {
		return e
	}
	if _, e = f.Write([]byte("1\n")); e != nil {
		return e
	}
	if _, e = f.Seek(0, 0); e != nil {
		return e
	}
	b, e := io.ReadAll(io.LimitReader(f, 16))
	if e != nil || strings.TrimSpace(string(b)) != "1" {
		return errors.New("disable_ipv6 readback failed")
	}
	if e = ns.checkCurrent(); e != nil {
		return e
	}
	return verifyLink()
}
