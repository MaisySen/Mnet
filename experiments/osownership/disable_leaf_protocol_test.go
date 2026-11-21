//go:build mnet_enrollment_development

package osownership

import (
	"errors"
	"io"
	"testing"
)

type disableLeafMemory struct {
	value                          string
	pos                            int
	reads, writes, closes          int
	failRead, failWrite, failClose bool
	shortWrite                     bool
}

func (f *disableLeafMemory) Read(p []byte) (int, error) {
	f.reads++
	if f.failRead {
		return 0, io.ErrUnexpectedEOF
	}
	if f.pos >= len(f.value) {
		return 0, io.EOF
	}
	n := copy(p, f.value[f.pos:])
	f.pos += n
	return n, nil
}
func (f *disableLeafMemory) Write(p []byte) (int, error) {
	f.writes++
	if f.failWrite {
		return 0, io.ErrClosedPipe
	}
	if f.shortWrite {
		return 1, nil
	}
	f.value = string(p)
	f.pos = len(p)
	return len(p), nil
}
func (f *disableLeafMemory) Seek(off int64, whence int) (int64, error) {
	if off != 0 || whence != io.SeekStart {
		return 0, ErrConflict
	}
	f.pos = 0
	return 0, nil
}
func (f *disableLeafMemory) Close() error {
	f.closes++
	if f.failClose {
		return io.ErrClosedPipe
	}
	return nil
}
func TestOwnedDisableLeafReadsBeforeSingleWrite(t *testing.T) {
	for _, v := range []string{"0\n", "1\n"} {
		f := &disableLeafMemory{value: v}
		checks := 0
		e := disableOwnedLeafOnce(f, func() error { checks++; return nil })
		expected := 1
		if v == "1\n" {
			expected = 0
		}
		if e != nil || f.writes != expected || f.closes != 1 || checks != 3 || f.value != "1\n" {
			t.Fatal(v, e, f, checks)
		}
	}
}
func TestOwnedDisableLeafAmbiguousWriteNeverRetries(t *testing.T) {
	for _, short := range []bool{false, true} {
		f := &disableLeafMemory{value: "0\n", failWrite: !short, shortWrite: short}
		e := disableOwnedLeafOnce(f, func() error { return nil })
		if e == nil || f.writes != 1 || f.closes != 1 {
			t.Fatal("ambiguous retry/success", e, f)
		}
		if short && !errors.Is(e, io.ErrShortWrite) {
			t.Fatal("short cause lost", e)
		}
	}
}
func TestOwnedDisableLeafPreReadAndCheckFailuresNeverWrite(t *testing.T) {
	for _, mode := range []string{"read", "invalid", "huge", "namespace-before", "namespace-after-read"} {
		f := &disableLeafMemory{value: "0\n"}
		checks := 0
		switch mode {
		case "read":
			f.failRead = true
		case "invalid":
			f.value = "2\n"
		case "huge":
			f.value = "00000000000000000"
		}
		e := disableOwnedLeafOnce(f, func() error {
			checks++
			if mode == "namespace-before" || mode == "namespace-after-read" && checks == 2 {
				return ErrConflict
			}
			return nil
		})
		if e == nil || f.writes != 0 || f.closes != 1 {
			t.Fatal(mode, e, f)
		}
	}
}
func TestOwnedDisableLeafCloseFailureRetainedForNoopAndWrite(t *testing.T) {
	for _, v := range []string{"0\n", "1\n"} {
		f := &disableLeafMemory{value: v, failClose: true}
		if e := disableOwnedLeafOnce(f, func() error { return nil }); !errors.Is(e, io.ErrClosedPipe) || f.closes != 1 {
			t.Fatal("close lost", e, f)
		}
	}
}
