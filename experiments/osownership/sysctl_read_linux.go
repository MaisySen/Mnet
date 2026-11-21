//go:build linux && mnet_enrollment_development && mnet_os_process_launch

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
	if !kernelExecutionReleased && !readOnlyEvidenceBuild {
		return nil, ErrExecutorUnreleased
	}
	out := SysctlSnapshot{}
	root, e := unix.Open("/proc/sys/net", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(root)
	paths := []string{"ipv4/ip_forward"}
	for _, scope := range []string{"all", "default"} {
		for _, leaf := range effectiveV4Leaves {
			paths = append(paths, "ipv4/conf/"+scope+"/"+leaf)
		}
	}
	names := []string{"all", "default"}
	for _, l := range raw.Links {
		if !safeInterfaceName(l.Name) {
			return nil, ErrConflict
		}
		names = append(names, l.Name)
	}
	for _, n := range names {
		paths = append(paths, "ipv4/conf/"+n+"/forwarding")
		applicable := true
		for _, l := range raw.Links {
			if l.Name == n {
				applicable, e = LinkIPv6Applicable(l)
				if e != nil {
					return nil, e
				}
			}
		}
		if e = ns.checkCurrent(); e != nil {
			return nil, e
		}
		dirfd, dirErr := openRelativeDirectory(root, []string{"ipv6", "conf", n})
		if dirErr != nil {
			if dirErr != unix.ENOENT {
				return nil, fmt.Errorf("ipv6 directory %s errno: %w", n, dirErr)
			}
			out[ipv6DirPrefix+n] = "ENOENT"
			if applicable {
				return nil, fmt.Errorf("IPv6 AF_SPEC present but directory absent: %s", n)
			}
		} else {
			unix.Close(dirfd)
			out[ipv6DirPrefix+n] = "present"
			if !applicable {
				return nil, fmt.Errorf("IPv6 directory present but AF_SPEC absent: %s", n)
			}
			for _, leaf := range []string{"forwarding", "force_forwarding", "disable_ipv6", "addr_gen_mode"} {
				paths = append(paths, "ipv6/conf/"+n+"/"+leaf)
			}
		}
		if e = ns.checkCurrent(); e != nil {
			return nil, e
		}
	}
	// Full nonsecret ALL/default IPv6 policy operands, exact source allowlist.
	// Missing CONFIG-dependent leaves are explicit unsupported-profile errors.
	for _, path := range ipv6PolicyPaths() {
		paths = append(paths, strings.TrimPrefix(path, "/proc/sys/net/"))
	}
	for _, path := range paths {
		if !permittedIPv6OperandPath(path) {
			return nil, afPolicy
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if e = ns.checkCurrent(); e != nil {
			return nil, e
		}
		parts := strings.Split(path, "/")
		fd, e := unix.FcntlInt(uintptr(root), unix.F_DUPFD_CLOEXEC, 0)
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
	if _, e := parseEffectiveV4(out); e != nil {
		return nil, e
	}
	if _, e := parseIPv6PolicyOperands(out); e != nil {
		return nil, e
	}
	return out, validateForwarding(raw, out)
}

// Resolve from trusted root without symlink traversal. ENOENT is returned as
// the actual syscall result; EPERM/ELOOP/etc are never absence evidence.
func openRelativeDirectory(root int, parts []string) (int, error) {
	fd, e := unix.FcntlInt(uintptr(root), unix.F_DUPFD_CLOEXEC, 0)
	if e != nil {
		return -1, e
	}
	for _, part := range parts {
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	return fd, nil
}
