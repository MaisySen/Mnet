//go:build mnet_enrollment_development

package osownership

// Strict finite rtnetlink framing. Pure codec plus mechanically guarded Linux
// transport source do not constitute a live Kernel executor or kernel proof.
import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"syscall"
)

const netlinkHeader = 16
const maxNetlinkBytes = 1 << 20
const nlError = 2
const nlDone = 3

// Datagrams must be from the kernel (sockaddr_nl pid0). Implementations must
// expose recvmsg truncation; absent dump consistency flags cannot mean complete.
type Datagram struct {
	Bytes     []byte
	SenderPID uint32
	Truncated bool
}
type NetlinkWire interface {
	Exchange(context.Context, []byte) ([]Datagram, error)
}
type NetlinkClient struct {
	mu       sync.Mutex
	wire     NetlinkWire
	sequence uint32
	localPID uint32
}

func NewInMemoryNetlinkClient(w NetlinkWire, localPID uint32) *NetlinkClient {
	return &NetlinkClient{wire: w, localPID: localPID}
}
func (c *NetlinkClient) requestFixture(ctx context.Context, kind uint16, payload []byte, dump bool) ([][]byte, error) {
	flags := uint16(1 | 4 | 0x200 | 0x400)
	if dump {
		flags = 1 | 0x300
		header, e := DumpRequestHeader(kind)
		if e != nil {
			return nil, e
		}
		if len(payload) == 0 {
			payload = header
		}
		if len(payload) != len(header) {
			return nil, errors.New("invalid dump request header")
		}
		for _, v := range payload {
			if v != 0 {
				return nil, errors.New("only AF_UNSPEC dump allowed")
			}
		}
	}
	return c.request(ctx, kind, payload, dump, flags, false)
}
func (c *NetlinkClient) request(ctx context.Context, kind uint16, payload []byte, dump bool, flags uint16, mutation bool) (out [][]byte, err error) {
	if c == nil {
		return nil, errors.New("nil client")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		{
			if closer, ok := c.wire.(interface{ Close() error }); ok {
				err = errors.Join(err, closer.Close())
			}
		}
	}()
	if !mutation && ((!dump && kind != 16 && kind != 20 && kind != 24) || (dump && kind != 18 && kind != 22 && kind != 26 && kind != 34)) {
		return nil, errors.New("unsupported netlink operation kind")
	}
	if c == nil || c.wire == nil || len(payload) > 65520 || ctx.Err() != nil {
		return nil, errors.New("invalid finite netlink request")
	}
	if c.sequence == ^uint32(0) {
		return nil, errors.New("netlink sequence exhausted")
	}
	c.sequence++
	seq := c.sequence
	request := make([]byte, netlinkHeader+len(payload))
	binary.NativeEndian.PutUint32(request, uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:], kind)

	binary.NativeEndian.PutUint16(request[6:], flags)
	binary.NativeEndian.PutUint32(request[8:], seq)
	binary.NativeEndian.PutUint32(request[12:], c.localPID)
	copy(request[16:], payload)
	ds, e := c.wire.Exchange(ctx, request)
	if e != nil {
		return nil, e
	}
	total := 0
	terminal := false
	var messages [][]byte
	for _, d := range ds {
		total += len(d.Bytes)
		if total > maxNetlinkBytes || d.Truncated || d.SenderPID != 0 {
			return nil, errors.New("untrusted/truncated netlink response")
		}
		b := d.Bytes
		for len(b) > 0 {
			if terminal || len(b) < netlinkHeader {
				return nil, errors.New("trailing or short netlink response")
			}
			n := int(binary.NativeEndian.Uint32(b))
			if n < netlinkHeader || n > len(b) {
				return nil, errors.New("invalid netlink length")
			}
			headerPID := binary.NativeEndian.Uint32(b[12:])
			if binary.NativeEndian.Uint32(b[8:]) != seq || headerPID != c.localPID {
				return nil, errors.New("netlink sequence/recipient mismatch")
			}
			typ := binary.NativeEndian.Uint16(b[4:])
			fl := binary.NativeEndian.Uint16(b[6:])
			if fl&0x10 != 0 {
				return nil, errors.New("interrupted dump")
			}
			body := b[16:n]
			switch typ {
			case nlError:
				if len(body) < 20 {
					return nil, errors.New("invalid ACK")
				}
				if binary.NativeEndian.Uint32(body[4:]) != uint32(len(request)) || binary.NativeEndian.Uint16(body[8:]) != kind || binary.NativeEndian.Uint16(body[10:]) != flags || binary.NativeEndian.Uint32(body[12:]) != seq || binary.NativeEndian.Uint32(body[16:]) != c.localPID {
					return nil, errors.New("ACK original request mismatch")
				}
				code := int32(binary.NativeEndian.Uint32(body))
				if code != 0 {
					return nil, fmt.Errorf("kernel negative ACK: %w", syscall.Errno(-int64(code)))
				}
				if dump {
					return nil, errors.New("unexpected success ACK for dump")
				}
				terminal = true
			case nlDone:
				if !dump || len(body) != 4 || binary.NativeEndian.Uint32(body) != 0 {
					return nil, errors.New("invalid dump completion")
				}
				terminal = true
			default:
				expected := map[uint16]uint16{18: 16, 22: 20, 26: 24, 34: 32}[kind]
				if !dump || fl&2 == 0 || typ != expected {
					return nil, errors.New("unexpected nonmultipart payload")
				}
				messages = append(messages, append([]byte(nil), body...))
			}
			aligned := (n + 3) &^ 3
			if aligned > len(b) {
				return nil, errors.New("netlink alignment overrun")
			}
			for _, pad := range b[n:aligned] {
				if pad != 0 {
					return nil, errors.New("nonzero padding")
				}
			}
			b = b[aligned:]
		}
	}
	if !terminal {
		return nil, errors.New("no kernel completion ACK")
	}
	return messages, nil
}

// Mutation flags are operation-specific. Delete is ACK-only, never CREATE or
// REPLACE. Link update is ACK-only. Address/route additions are exclusive.
func mutationRequestFlags(kind uint16) (uint16, error) {
	flags := uint16(1 | 4)
	switch kind {
	case 20, 24:
		flags |= 0x200 | 0x400
	case 16, 21, 25:
	default:
		return 0, errors.New("unsupported mutation kind")
	}
	return flags, nil
}
func (c *NetlinkClient) mutate(ctx context.Context, kind uint16, payload []byte) error {
	flags, e := mutationRequestFlags(kind)
	if e != nil {
		return e
	}
	_, e = c.request(ctx, kind, payload, false, flags, true)
	return e
}

// Exact family-unspecified request structure for each dump, no guessed header.
func DumpRequestHeader(kind uint16) ([]byte, error) {
	switch kind {
	case 18:
		return make([]byte, 16), nil
	case 22:
		return make([]byte, 8), nil
	case 26, 34:
		return make([]byte, 12), nil
	}
	return nil, errors.New("unsupported dump header")
}

// Request is exclusively a dump entry point. Mutations use opaque exact
// constructors, never generic public byte+kind submission.
func (c *NetlinkClient) Request(ctx context.Context, kind uint16, payload []byte, dump bool) ([][]byte, error) {
	if !dump {
		return nil, errors.New("raw mutation API refused")
	}
	return c.requestFixture(ctx, kind, payload, true)
}
