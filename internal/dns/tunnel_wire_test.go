package dns

import (
	"encoding/binary"
	"fmt"
	"io"
	"runtime"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

// Independent wire checksum, not the production stack's validation helper.
func wireChecksum(p []byte) uint16 {
	var sum uint32
	for len(p) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(p))
		p = p[2:]
	}
	if len(p) > 0 {
		sum += uint32(p[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return ^uint16(sum)
}
func udpPseudo(packet []byte) []byte {
	pseudo := append([]byte(nil), packet[12:20]...)
	pseudo = append(pseudo, 0, 17, packet[24], packet[25])
	return append(pseudo, packet[20:]...)
}
func TestTunnelUDPReplyWireTupleLengthAndChecksums(t *testing.T) {
	for _, source := range [][4]byte{{100, 64, 0, 1}, {100, 127, 255, 254}} {
		for _, port := range []uint16{1024, 32768, 65535} {
			t.Run(fmt.Sprintf("%v/%d", source, port), func(t *testing.T) {
				replies := make(chan []byte, 8)
				raw := &testRawTUN{in: make(chan []byte, 1), done: make(chan struct{}), out: func(p []byte) { replies <- p }}
				tunnel := NewTunnel(raw)
				s, _ := New(LocalIPv4)
				s.Update(testMap())
				if err := tunnel.StartDNS(s, 1280); err != nil {
					t.Fatal(err)
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, err := tunnel.Read([][]byte{make([]byte, 2048)}, []int{0}, 0)
					if err != io.EOF {
						t.Errorf("read shutdown: %v", err)
					}
				}()
				t.Cleanup(func() { tunnel.Close(); <-done })
				packet, q := wireQuery(t, source, port)
				raw.in <- packet
				select {
				case response := <-replies:
					if len(response) < 28 || response[0] != 0x45 || response[9] != 17 || int(binary.BigEndian.Uint16(response[2:4])) != len(response) {
						t.Fatal("invalid IP reply length/header")
					}
					if wireChecksum(response[:20]) != 0 {
						t.Fatal("invalid IP checksum")
					}
					for i := 0; i < 4; i++ {
						if response[12+i] != packet[16+i] || response[16+i] != source[i] {
							t.Fatal("reply address tuple mismatch")
						}
					}
					if binary.BigEndian.Uint16(response[20:22]) != 53 || binary.BigEndian.Uint16(response[22:24]) != port || int(binary.BigEndian.Uint16(response[24:26])) != len(response)-20 {
						t.Fatal("reply UDP tuple/length mismatch")
					}
					if binary.BigEndian.Uint16(response[26:28]) == 0 || wireChecksum(udpPseudo(response)) != 0 {
						t.Fatal("missing or invalid UDP checksum")
					}
					deadline := time.Now().Add(time.Second)
					for !tunnel.Diagnostics()["reply_written"] && time.Now().Before(deadline) {
						runtime.Gosched()
					}
					if !tunnel.Diagnostics()["reply_written"] || tunnel.Diagnostics()["reply_write_failed"] {
						t.Fatal("reply write diagnostics wrong")
					}
					var answer mdns.Msg
					if err := answer.Unpack(response[28:]); err != nil || answer.Id != q.Id || !answer.Response || len(answer.Answer) != 1 {
						t.Fatalf("invalid DNS wire answer: %v", err)
					}
					if a, ok := answer.Answer[0].(*mdns.A); !ok || a.A.String() != "100.64.0.2" {
						t.Fatal("wrong DNS address")
					}
				case <-time.After(2 * time.Second):
					t.Fatal("no wire reply")
				}
			})
		}
	}
}

func wireQuery(t *testing.T, source [4]byte, port uint16) ([]byte, *mdns.Msg) {
	t.Helper()
	q := new(mdns.Msg)
	q.SetQuestion("server.home.mnet.", mdns.TypeA)
	q.Id = 0x4d4e
	payload, err := q.Pack()
	if err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 28+len(payload))
	packet[0] = 0x45
	packet[8] = 64
	packet[9] = 17
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	copy(packet[12:16], source[:])
	copy(packet[16:20], []byte{198, 18, 0, 53})
	binary.BigEndian.PutUint16(packet[20:22], port)
	binary.BigEndian.PutUint16(packet[22:24], 53)
	binary.BigEndian.PutUint16(packet[24:26], uint16(8+len(payload)))
	copy(packet[28:], payload)
	binary.BigEndian.PutUint16(packet[10:12], wireChecksum(packet[:20]))
	binary.BigEndian.PutUint16(packet[26:28], wireChecksum(udpPseudo(packet)))

	return packet, q
}

type failedReplyTUN struct {
	*testRawTUN
	short bool
}

func (f *failedReplyTUN) Write(_ [][]byte, _ int) (int, error) {
	if f.short {
		return 0, nil
	}
	return 0, io.ErrClosedPipe
}
func TestTunnelReplyWriteFailureIsNotSuccess(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			raw := &testRawTUN{in: make(chan []byte, 1), done: make(chan struct{}), out: func([]byte) { t.Error("unexpected write") }}
			tunnel := NewTunnel(&failedReplyTUN{raw, short})
			s, _ := New(LocalIPv4)
			s.Update(testMap())
			if err := tunnel.StartDNS(s, 1280); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { defer close(done); tunnel.Read([][]byte{make([]byte, 2048)}, []int{0}, 0) }()
			t.Cleanup(func() { tunnel.Close(); <-done })
			packet, _ := wireQuery(t, [4]byte{100, 64, 0, 1}, 32768)
			raw.in <- packet
			deadline := time.Now().Add(time.Second)
			for !tunnel.Diagnostics()["reply_write_failed"] && time.Now().Before(deadline) {
				runtime.Gosched()
			}
			flags := tunnel.Diagnostics()
			if !flags["reply_write_failed"] || flags["reply_written"] {
				t.Fatal("failed write reported successful")
			}
		})
	}
}
