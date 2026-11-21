//go:build mnet_enrollment_development

package osownership

import (
	"errors"
	"io"
	"strings"
)

// Exact per-leaf IO protocol, NOT transition completion. No caller can obtain
// post-disable evidence from this function. Independent AF/raw verification is
// still required, and unknown expected-diff profiles must refuse externally.
const ownedDisableDiffIncomplete ownedAFRefusal = "owned-disable-complete-expected-diff-unresolved"

type ownedDisableLeaf interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
}

func disableOwnedLeafOnce(f ownedDisableLeaf, check func() error) (err error) {
	if f == nil {
		return ErrConflict
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if check == nil {
		return ErrConflict
	}
	read := func() (string, error) {
		if _, e := f.Seek(0, io.SeekStart); e != nil {
			return "", e
		}
		b, e := io.ReadAll(io.LimitReader(f, 17))
		if e != nil {
			return "", e
		}
		if len(b) > 16 {
			return "", ErrConflict
		}
		v := strings.TrimSpace(string(b))
		if v != "0" && v != "1" {
			return "", ErrConflict
		}
		return v, nil
	}
	if e := check(); e != nil {
		return e
	}
	before, e := read()
	if e != nil {
		return e
	}
	if e = check(); e != nil {
		return e
	}
	if before == "0" {
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			return e
		}
		// Single attempt, never retry an error/short/ambiguous write.
		n, e := f.Write([]byte("1\n"))
		if e != nil {
			return e
		}
		if n != 2 {
			return io.ErrShortWrite
		}
	}
	after, e := read()
	if e != nil {
		return e
	}
	if after != "1" {
		return ErrConflict
	}
	return check()
}
