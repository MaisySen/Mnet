//go:build mnet_enrollment_development && (linux || darwin || freebsd || openbsd || netbsd || dragonfly)

package daemon

import (
	"os"
	"syscall"
)

func developmentOwnedByEffectiveUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && os.Geteuid() >= 0 && stat.Uid == uint32(os.Geteuid())
}
