//go:build mnet_enrollment_development && !(linux || darwin || freebsd || openbsd || netbsd || dragonfly)

package daemon

import "os"

// This disposable entry is unavailable until owner verification is supported.
func developmentOwnedByEffectiveUser(os.FileInfo) bool { return false }
