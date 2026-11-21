//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight

package osownership

// Inert owner-side work function, deliberately not dispatched by the existing
// evidence binary. Extra tag is isolation, never execution authorization.
import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"reflect"
)

type DeviceMetadata struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Rdev   uint64 `json:"rdev"`
	Mode   uint32 `json:"mode"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
}
type DevicePreflightEvidence struct {
	Version              int             `json:"version"`
	Path                 string          `json:"path"`
	Namespace            Namespace       `json:"namespace"`
	Metadata             *DeviceMetadata `json:"metadata,omitempty"`
	OpenedMetadata       *DeviceMetadata `json:"opened_metadata,omitempty"`
	Outcome              string          `json:"outcome"`
	Errno                int             `json:"errno,omitempty"`
	FD                   int             `json:"fd"`
	OpenAttempted        bool            `json:"open_attempted"`
	CloseAttempted       bool            `json:"close_attempted"`
	Closed               bool            `json:"closed"`
	CLOEXEC              bool            `json:"cloexec"`
	Nonblock             bool            `json:"nonblock"`
	FDsBefore            []int           `json:"fds_before"`
	FDsAfter             []int           `json:"fds_after"`
	FDInventoryUnchanged bool            `json:"fd_inventory_unchanged"`
	BaselineHash         string          `json:"baseline_hash"`
	FinalHash            string          `json:"final_hash"`
	BaselineUnchanged    bool            `json:"baseline_unchanged"`
	NamespaceRechecked   bool            `json:"namespace_rechecked"`
}

const preflightDevicePath = "/dev/net/tun"
const preflightOpenFlags = unix.O_RDWR | unix.O_NONBLOCK | unix.O_CLOEXEC | unix.O_NOFOLLOW

type devicePreflightOps interface {
	CheckNamespace() error
	Metadata() (DeviceMetadata, error)
	Open() (int, error)
	Fstat(int) (DeviceMetadata, error)
	FDFlags(int) (int, error)
	StatusFlags(int) (int, error)
	Close(int) error
	Inventory() ([]int, error)
}

func deviceResultForError(e error) (string, int, bool) {
	var n unix.Errno
	if !errors.As(e, &n) {
		return "error", 0, false
	}
	switch n {
	case unix.ENOENT:
		return "missing", int(n), true
	case unix.EACCES, unix.EPERM:
		return "denied", int(n), true
	}
	return "error", int(n), false
}
func isTUNDevice(m DeviceMetadata) bool {
	return m.Mode&unix.S_IFMT == unix.S_IFCHR && unix.Major(m.Rdev) == 10 && unix.Minor(m.Rdev) == 200
}
func captureDevicePreflight(ops devicePreflightOps, namespace Namespace) (r DevicePreflightEvidence, err error) {
	r = DevicePreflightEvidence{Version: 1, Path: preflightDevicePath, Namespace: namespace, FD: -1, Outcome: "error"}
	if err = ops.CheckNamespace(); err != nil {
		return
	}
	r.FDsBefore, err = ops.Inventory()
	if err != nil {
		return
	}
	// Even missing/denied outcomes require after-state oracles. No access guess.
	defer func() {
		after, e := ops.Inventory()
		r.FDsAfter = after
		r.FDInventoryUnchanged = e == nil && reflect.DeepEqual(r.FDsBefore, after)
		err = errors.Join(err, e)
		if !r.FDInventoryUnchanged {
			err = errors.Join(err, ErrConflict)
		}
		e = ops.CheckNamespace()
		r.NamespaceRechecked = e == nil
		err = errors.Join(err, e)
	}()
	m, e := ops.Metadata()
	if e != nil {
		out, n, valid := deviceResultForError(e)
		r.Outcome = out
		r.Errno = n
		if !valid {
			err = e
		}
		return
	}
	r.Metadata = &m
	// Never follow symlinks, open arbitrary objects or adopt another device.
	if !isTUNDevice(m) {
		r.Outcome = "refused-object"
		return
	}
	if err = ops.CheckNamespace(); err != nil {
		return
	}
	r.OpenAttempted = true
	fd, e := ops.Open()
	if e != nil {
		out, n, valid := deviceResultForError(e)
		r.Outcome = out
		r.Errno = n
		if !valid {
			err = e
		}
		return
	}
	r.FD = fd
	if fd < 0 {
		err = ErrConflict
		return
	}
	// No retry on close error, descriptor-number reuse must never be hit.
	defer func() {
		r.CloseAttempted = true
		e := ops.Close(fd)
		r.Closed = e == nil
		err = errors.Join(err, e)
		if e != nil {
			r.Outcome = "close-error"
		}
	}()
	fm, e := ops.Fstat(fd)
	if e != nil {
		err = e
		return
	}
	r.OpenedMetadata = &fm
	if fm != m || !isTUNDevice(fm) {
		err = ErrConflict
		r.Outcome = "identity-conflict"
		return
	}
	flags, e := ops.FDFlags(fd)
	if e != nil {
		err = e
		return
	}
	r.CLOEXEC = flags&unix.FD_CLOEXEC != 0
	if !r.CLOEXEC {
		err = ErrConflict
		r.Outcome = "cloexec-missing"
		return
	}
	if err = ops.CheckNamespace(); err != nil {
		return
	}
	status, e := ops.StatusFlags(fd)
	if e != nil {
		err = e
		return
	}
	r.Nonblock = status&unix.O_NONBLOCK != 0
	if !r.Nonblock {
		err = ErrConflict
		r.Outcome = "nonblock-missing"
		return
	}
	r.Outcome = "opened-closed"
	return
}

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
func runDevicePreflightSource(ctx context.Context, ns *namespaceCustody) (r DevicePreflightEvidence, err error) {
	if ns == nil || !ns.identity.Isolated {
		return r, ErrConflict
	}
	if err = ctx.Err(); err != nil {
		return
	}
	if err = ns.checkCurrent(); err != nil {
		return
	}
	raw, err := ReadStableInventory(ctx, residentNetlink{ns}, ns.checkCurrent)
	if err != nil {
		return
	}
	sys, err := readPolicySysctlsSource(ctx, ns, raw)
	if err != nil {
		return
	}
	initial := bootstrapEvidenceDigest(raw, sys)
	r, err = captureDevicePreflight(residentDevicePreflight{ns}, ns.identity)
	r.BaselineHash = initial
	after, e := ReadStableInventory(ctx, residentNetlink{ns}, ns.checkCurrent)
	err = errors.Join(err, e)
	if e != nil {
		return
	}
	afterSys, e := readPolicySysctlsSource(ctx, ns, after)
	err = errors.Join(err, e)
	if e != nil {
		return
	}
	r.FinalHash = bootstrapEvidenceDigest(after, afterSys)
	r.BaselineUnchanged = initial == r.FinalHash
	if !r.BaselineUnchanged {
		err = errors.Join(err, ErrConflict)
	}
	return
}
