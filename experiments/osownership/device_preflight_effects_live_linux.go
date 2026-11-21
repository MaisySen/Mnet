//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight && mnet_os_preflight_launch && mnet_os_process_launch

package osownership

import (
	"context"
	"golang.org/x/sys/unix"
)

type residentDevicePreflight struct{ ns *namespaceCustody }

func (o residentDevicePreflight) CheckNamespace() error { return o.ns.checkCurrent() }
func metadataFromStat(s unix.Stat_t) DeviceMetadata {
	return DeviceMetadata{uint64(s.Dev), s.Ino, uint64(s.Rdev), s.Mode, s.Uid, s.Gid}
}
func (o residentDevicePreflight) Metadata() (DeviceMetadata, error) {
	var s unix.Stat_t
	e := unix.Lstat(preflightDevicePath, &s)
	return metadataFromStat(s), e
}
func (o residentDevicePreflight) Open() (int, error) {
	return unix.Open(preflightDevicePath, preflightOpenFlags, 0)
}
func (o residentDevicePreflight) Fstat(fd int) (DeviceMetadata, error) {
	var s unix.Stat_t
	e := unix.Fstat(fd, &s)
	return metadataFromStat(s), e
}
func (o residentDevicePreflight) FDFlags(fd int) (int, error) {
	return unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
}
func (o residentDevicePreflight) StatusFlags(fd int) (int, error) {
	return unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
}
func (o residentDevicePreflight) Close(fd int) error        { return unix.Close(fd) }
func (o residentDevicePreflight) Inventory() ([]int, error) { return clientFDInventorySource() }

// Only receives held fresh namespace custody established by reviewed supervisor.
// No launcher/dispatch added. An outer caller must separately capture unchanged
// outer namespace and Wait/custody evidence before any execution can be reviewed.
func runDevicePreflightSource(ctx context.Context, ns *namespaceCustody) (DevicePreflightEvidence, error) {
	if ns == nil {
		return DevicePreflightEvidence{}, ErrConflict
	}
	snapshot := func() (string, error) {
		if e := ctx.Err(); e != nil {
			return "", e
		}
		if e := ns.checkCurrent(); e != nil {
			return "", e
		}
		raw, e := ReadStableInventory(ctx, residentNetlink{ns: ns}, ns.checkCurrent)
		if e != nil {
			return "", e
		}
		sys, e := readPolicySysctlsSource(ctx, ns, raw)
		if e != nil {
			return "", e
		}
		return bootstrapEvidenceDigest(raw, sys), nil
	}
	return runDevicePreflightBoundedSource(ns.identity, residentDevicePreflight{ns}, snapshot)
}
