//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import "strings"

// Required owned disable evidence is independent of optional CONFIG diagnostic
// collection. This source boundary still has NO complete predicted AF/raw diff
// or independently bound collector. Unknown diff refuses BEFORE opening/writing
// any leaf, including an already1 no-op. No post-disable result can be minted.
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
	return ownedDisableDiffIncomplete
}
