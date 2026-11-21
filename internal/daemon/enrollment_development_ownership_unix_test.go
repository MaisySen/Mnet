//go:build mnet_enrollment_development && (linux || darwin || freebsd || openbsd || netbsd || dragonfly)

package daemon

import (
	"os"
	"syscall"
	"testing"
)

type developmentForeignOwnerInfo struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (i developmentForeignOwnerInfo) Sys() any { return &i.stat }
func TestDevelopmentRootOwnerIsEffectiveUser(t *testing.T) {
	info, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !developmentOwnedByEffectiveUser(info) {
		t.Fatal("actual owned root refused")
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid++
	if developmentOwnedByEffectiveUser(developmentForeignOwnerInfo{info, stat}) {
		t.Fatal("foreign UID accepted despite private mode")
	}
}
