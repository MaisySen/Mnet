//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

type fakeWire struct {
	mutate  func([]byte) Datagram
	request []byte
}

func (w *fakeWire) Exchange(_ context.Context, b []byte) ([]Datagram, error) {
	w.request = append([]byte(nil), b...)
	reply := make([]byte, 36)
	binary.NativeEndian.PutUint32(reply, 36)
	binary.NativeEndian.PutUint16(reply[4:], nlError)
	copy(reply[8:16], b[8:16])
	copy(reply[20:36], b[:16])
	d := Datagram{Bytes: reply}
	if w.mutate != nil {
		d = w.mutate(reply)
	}
	return []Datagram{d}, nil
}
func TestNetlinkExclusiveACKAndNegativeFrames(t *testing.T) {
	w := &fakeWire{}
	c := NewInMemoryNetlinkClient(w, 123)
	if _, e := c.requestFixture(context.Background(), 16, nil, false); e != nil {
		t.Fatal(e)
	}
	if binary.NativeEndian.Uint16(w.request[6:]) != 1|4|0x200|0x400 {
		t.Fatal("exclusive create flags missing")
	}
	for _, name := range []string{"sender", "sequence", "recipient", "negative", "truncated", "short", "missing", "dump-interrupted"} {
		t.Run(name, func(t *testing.T) {
			w := &fakeWire{mutate: func(b []byte) Datagram {
				d := Datagram{Bytes: b}
				switch name {
				case "sender":
					d.SenderPID = 99
				case "sequence":
					b[8]++
				case "recipient":
					b[12]++
				case "negative":
					binary.NativeEndian.PutUint32(b[16:], ^uint32(0))
				case "truncated":
					d.Truncated = true
				case "short":
					d.Bytes = b[:5]
				case "missing":
					d.Bytes = nil
				case "dump-interrupted":
					binary.NativeEndian.PutUint16(b[6:], 0x10)
				}
				return d
			}}
			if _, e := NewInMemoryNetlinkClient(w, 123).requestFixture(context.Background(), 16, nil, false); e == nil {
				t.Fatal("accepted invalid ACK")
			}
		})
	}
}
func TestNetlinkTransportTimeoutNotSuccess(t *testing.T) {
	c := NewInMemoryNetlinkClient(timeoutWire{}, 1)
	if _, e := c.requestFixture(context.Background(), 16, nil, false); e == nil {
		t.Fatal("timeout treated as ACK")
	}
}

type timeoutWire struct{}

func (timeoutWire) Exchange(context.Context, []byte) ([]Datagram, error) {
	return nil, errors.New("timeout after possible success")
}

type dumpWire struct{ interrupted, missing bool }

func (w dumpWire) Exchange(_ context.Context, r []byte) ([]Datagram, error) {
	m := make([]byte, 20)
	binary.NativeEndian.PutUint32(m, 20)
	binary.NativeEndian.PutUint16(m[4:], 16)
	binary.NativeEndian.PutUint16(m[6:], 2)
	copy(m[8:16], r[8:16])
	m[16] = 42
	if w.interrupted {
		binary.NativeEndian.PutUint16(m[6:], 2|0x10)
	}
	if w.missing {
		return []Datagram{{Bytes: m}}, nil
	}
	done := make([]byte, 20)
	binary.NativeEndian.PutUint32(done, 20)
	binary.NativeEndian.PutUint16(done[4:], nlDone)
	copy(done[8:16], r[8:16])
	return []Datagram{{Bytes: append(m, done...)}}, nil
}
func TestNetlinkMultipartDumpRequiresUninterruptedCompletion(t *testing.T) {
	for _, w := range []dumpWire{{}, {missing: true}, {interrupted: true}} {
		c := NewInMemoryNetlinkClient(w, 123)
		ms, e := c.requestFixture(context.Background(), 18, nil, true)
		if !w.missing && !w.interrupted {
			if e != nil || len(ms) != 1 || ms[0][0] != 42 {
				t.Fatal(ms, e)
			}
		} else if e == nil {
			t.Fatal("incomplete/interrupted dump accepted")
		}
	}
	if _, e := NewInMemoryNetlinkClient(&fakeWire{}, 1).requestFixture(context.Background(), 17, nil, false); e == nil {
		t.Fatal("live delete kind incorrectly allowed by create codec")
	}
}
