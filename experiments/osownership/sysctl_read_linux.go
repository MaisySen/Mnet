//go:build linux && mnet_enrollment_development

package osownership

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
)

// Read-only collector source shares locked namespace custody with the writer.
// Missing per-device IPv6 leaves produce an explicit applicability gap. No
// missing file is converted to disabled, unsupported or safe.
func readPolicySysctlsSource(ctx context.Context, ns *namespaceCustody, raw RawInventory) (SysctlSnapshot, error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	out := SysctlSnapshot{}
	root, e := unix.Open("/proc/sys/net", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(root)
	paths := []string{"ipv4/ip_forward"}
	names := []string{"all", "default"}
	for _, l := range raw.Links {
		if !safeInterfaceName(l.Name) {
			return nil, ErrConflict
		}
		names = append(names, l.Name)
	}
	for _, n := range names {
		paths = append(paths, "ipv4/conf/"+n+"/forwarding")
		for _, leaf := range []string{"forwarding", "disable_ipv6", "addr_gen_mode"} {
			paths = append(paths, "ipv6/conf/"+n+"/"+leaf)
		}
	}
	for _, path := range paths {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if e = ns.checkCurrent(); e != nil {
			return nil, e
		}
		parts := strings.Split(path, "/")
		fd, e := unix.Dup(root)
		if e != nil {
			return nil, e
		}
		for j, part := range parts {
			flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
			if j < len(parts)-1 {
				flags |= unix.O_DIRECTORY
			}
			next, err := unix.Openat(fd, part, flags, 0)
			unix.Close(fd)
			if err != nil {
				return nil, fmt.Errorf("sysctl applicability/read gap %s: %w", path, err)
			}
			fd = next
		}
		f := os.NewFile(uintptr(fd), "policy-sysctl-readback")
		b, readErr := io.ReadAll(io.LimitReader(f, 17))
		closeErr := f.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(b) > 16 {
			return nil, ErrConflict
		}
		out["/proc/sys/net/"+path] = strings.TrimSpace(string(b))
		if e = ns.checkCurrent(); e != nil {
			return nil, e
		}
	}
	return out, validateForwarding(raw, out)
}
