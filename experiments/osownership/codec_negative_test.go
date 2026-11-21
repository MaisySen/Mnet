//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"syscall"
	"testing"
)

type wireFunc func([]byte) ([]Datagram, error)

func (f wireFunc) Exchange(_ context.Context, b []byte) ([]Datagram, error) { return f(b) }
func ackFrame(r []byte, code int32) []byte {
	b := make([]byte, 36)
	binary.NativeEndian.PutUint32(b, 36)
	binary.NativeEndian.PutUint16(b[4:], 2)
	copy(b[8:16], r[8:16])
	binary.NativeEndian.PutUint32(b[16:], uint32(code))
	copy(b[20:], r[:16])
	return b
}
func dumpFrames(r []byte) ([]byte, []byte) {
	b := make([]byte, 20)
	binary.NativeEndian.PutUint32(b, 20)
	binary.NativeEndian.PutUint16(b[4:], 16)
	binary.NativeEndian.PutUint16(b[6:], 2)
	copy(b[8:16], r[8:16])
	done := make([]byte, 20)
	binary.NativeEndian.PutUint32(done, 20)
	binary.NativeEndian.PutUint16(done[4:], 3)
	copy(done[8:16], r[8:16])
	return b, done
}
func TestEachACKEmbeddedFieldAndTerminalCorruption(t *testing.T) {
	for _, name := range []string{"embedded-len", "embedded-kind", "embedded-flags", "embedded-seq", "embedded-pid", "trailing-frame", "trailing-byte", "length-overrun", "length-small", "alignment-overrun", "nonzero-padding"} {
		t.Run(name, func(t *testing.T) {
			w := wireFunc(func(r []byte) ([]Datagram, error) {
				b := ackFrame(r, 0)
				switch name {
				case "embedded-len":
					b[20]++
				case "embedded-kind":
					b[24]++
				case "embedded-flags":
					b[26]++
				case "embedded-seq":
					b[28]++
				case "embedded-pid":
					b[32]++
				case "trailing-frame":
					b = append(b, ackFrame(r, 0)...)
				case "trailing-byte":
					b = append(b, 0)
				case "length-overrun":
					binary.NativeEndian.PutUint32(b, 1000)
				case "length-small":
					binary.NativeEndian.PutUint32(b, 15)
				case "alignment-overrun":
					b = append(b, 0)
					binary.NativeEndian.PutUint32(b, 37)
				case "nonzero-padding":
					b = append(b, 0, 0, 0, 1)
					binary.NativeEndian.PutUint32(b, 37)
				}
				return []Datagram{{Bytes: b}}, nil
			})
			if _, e := NewInMemoryNetlinkClient(w, 55).Request(context.Background(), 16, nil, false); e == nil {
				t.Fatal("single corrupted ACK field accepted")
			}
		})
	}
}
func TestEachDumpCompletionAndMultipartCorruption(t *testing.T) {
	for _, name := range []string{"no-multipart", "wrong-kind", "done-nonzero", "done-short", "done-extra", "ack-success", "negative-ack", "missing-done", "post-done", "total-cap"} {
		t.Run(name, func(t *testing.T) {
			w := wireFunc(func(r []byte) ([]Datagram, error) {
				b, d := dumpFrames(r)
				switch name {
				case "no-multipart":
					binary.NativeEndian.PutUint16(b[6:], 0)
				case "wrong-kind":
					binary.NativeEndian.PutUint16(b[4:], 20)
				case "done-nonzero":
					d[16] = 1
				case "done-short":
					d = d[:16]
					binary.NativeEndian.PutUint32(d, 16)
				case "done-extra":
					d = append(d, 0, 0, 0, 0)
					binary.NativeEndian.PutUint32(d, 24)
				case "ack-success":
					return []Datagram{{Bytes: ackFrame(r, 0)}}, nil
				case "negative-ack":
					return []Datagram{{Bytes: ackFrame(r, -int32(syscall.EPERM))}}, nil
				case "missing-done":
					d = nil
				case "post-done":
					return []Datagram{{Bytes: d}, {Bytes: b}}, nil
				case "total-cap":
					large := make([]byte, maxNetlinkBytes+4)
					return []Datagram{{Bytes: large}}, nil
				}
				return []Datagram{{Bytes: b}, {Bytes: d}}, nil
			})
			_, e := NewInMemoryNetlinkClient(w, 55).Request(context.Background(), 18, nil, true)
			if e == nil {
				t.Fatal("dump corruption accepted")
			}
			if name == "negative-ack" && !errors.Is(e, syscall.EPERM) {
				t.Fatal("typed dump errno lost", e)
			}
		})
	}
}
func TestTypedErrnosAndValidMultiDatagram(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.EEXIST, syscall.EPERM, syscall.EBUSY, syscall.ENODEV} {
		w := wireFunc(func(r []byte) ([]Datagram, error) { return []Datagram{{Bytes: ackFrame(r, -int32(errno))}}, nil })
		if _, e := NewInMemoryNetlinkClient(w, 1).Request(context.Background(), 16, nil, false); !errors.Is(e, errno) {
			t.Fatal("typed errno lost", e)
		}
	}
	w := wireFunc(func(r []byte) ([]Datagram, error) {
		b, d := dumpFrames(r)
		return []Datagram{{Bytes: b}, {Bytes: b}, {Bytes: d}}, nil
	})
	ms, e := NewInMemoryNetlinkClient(w, 1).Request(context.Background(), 18, nil, true)
	if e != nil || len(ms) != 2 {
		t.Fatal("valid multidatagram refused", e)
	}
}
func TestDumpHeaderConstructionAndRefusal(t *testing.T) {
	for kind, size := range map[uint16]int{18: 16, 22: 8, 26: 12, 34: 12} {
		h, e := DumpRequestHeader(kind)
		if e != nil || len(h) != size {
			t.Fatal(kind, e)
		}
		w := wireFunc(func(r []byte) ([]Datagram, error) {
			if len(r) != 16+size {
				t.Fatal("wrong dump body size")
			}
			_, d := dumpFrames(r)
			return []Datagram{{Bytes: d}}, nil
		})
		c := NewInMemoryNetlinkClient(w, 1)
		if _, e = c.Request(context.Background(), kind, nil, true); e != nil {
			t.Fatal(e)
		}
		if _, e = c.Request(context.Background(), kind, []byte{2}, true); e == nil {
			t.Fatal("malformed header accepted")
		}
	}
}
