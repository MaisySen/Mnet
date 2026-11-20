package desk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// An exact pinned peer can still return malformed acknowledgements. No native
// backend is created here, and a refusal is never evidence of native release.
func TestWireClientMalformedAcknowledgementsAreTerminal(t *testing.T) {
	for _, stage := range []string{"ready", "input", "stop"} {
		for _, fault := range []string{"negative", "wrong-kind", "input", "binding", "zero", "oversize", "duplicate-key", "unknown-field"} {
			t.Run(stage+"/"+fault, func(t *testing.T) {
				layout, binding, primary, recipient := pairTransportFixture(t)
				a, b := tlsFixturePair(t, primary, recipient)
				defer a.NetConn().Close()
				defer b.NetConn().Close()
				done := make(chan error, 1)
				go func() {
					run := func() error {
						if err := authenticate(b, binding.Primary); err != nil {
							return err
						}
						hello, err := readWire(b)
						if err != nil || hello.Kind != "hello" || hello.Binding == nil || *hello.Binding != binding {
							return fmt.Errorf("fixture hello: %v", err)
						}
						if stage != "ready" {
							if err = writeWire(b, wireMessage{Kind: "ready", OK: true}); err != nil {
								return err
							}
							request, err := readWire(b)
							if err != nil || request.Kind != stage {
								return fmt.Errorf("fixture request %s: %v", request.Kind, err)
							}
						}
						ack := wireMessage{Kind: stage, OK: true}
						switch fault {
						case "negative":
							ack.OK = false
						case "wrong-kind":
							ack.Kind = "hello"
						case "input":
							ack.Input = &Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}
						case "binding":
							ack.Binding = &binding
						}
						switch fault {
						case "zero", "oversize":
							var header [4]byte
							if fault == "oversize" {
								binary.BigEndian.PutUint32(header[:], 4097)
							}
							if _, err = b.Write(header[:]); err != nil {
								return err
							}
						case "duplicate-key", "unknown-field":
							body := []byte(fmt.Sprintf(`{"kind":%q,"ok":true,"ok":true}`, stage))
							if fault == "unknown-field" {
								body = []byte(fmt.Sprintf(`{"kind":%q,"ok":true,"extra":1}`, stage))
							}
							var header [4]byte
							binary.BigEndian.PutUint32(header[:], uint32(len(body)))
							if _, err = b.Write(append(header[:], body...)); err != nil {
								return err
							}
						default:
							if err = writeWire(b, ack); err != nil {
								return err
							}
						}
						// A bad ACK must close transport, not retry input or send an optimistic
						// cleanup message after acknowledgement ownership has become uncertain.
						if err = b.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
							return err
						}
						var next [1]byte
						n, err := b.Read(next[:])
						if n != 0 || !errors.Is(err, io.EOF) {
							return fmt.Errorf("fixture peer did not close without more bytes: n=%d err=%v", n, err)
						}
						return nil
					}
					done <- run()
				}()
				client, err := ConnectPair(a, binding, layout, func() bool { return true })
				if stage == "ready" {
					if err == nil || client != nil {
						t.Fatal("malformed ready accepted")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if stage == "input" {
						err = client.Apply(Input{Sequence: 1, Kind: "key", Action: "down", Code: 38})
					} else {
						err = client.Release()
					}
					if err == nil {
						t.Fatal("malformed ACK became success")
					}
					first := err
					select {
					case <-client.Stopped():
					default:
						t.Fatal("malformed ACK left transport active")
					}
					for range 2 {
						if err = client.Release(); err == nil || !errors.Is(err, first) {
							t.Fatal("cleanup forgot uncertainty", err, first)
						}
						if err = client.Apply(Input{Sequence: 2, Kind: "key", Action: "up", Code: 38}); err == nil {
							t.Fatal("bad ACK allowed successor input")
						}
					}
				}
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("fixture server did not join")
				}
			})
		}
	}
}
