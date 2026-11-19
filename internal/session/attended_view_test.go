package session

import (
	"net"
	"testing"
)

func TestAttendedViewRefusesHeadlessBeforePrompt(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	called := make(chan bool, 1)
	done := make(chan *Mux, 1)
	go func() {
		m, _ := Listen(testAttendedListener{b}, "host", func(Hello, net.Addr) (bool, string) { called <- true; return true, "" })
		done <- m
	}()
	client := newMux(a, StreamFile)
	if err := client.sendJSON(StreamControl, msgOpen, PriInteractive, map[string]any{"version": protoVersion, "nodeName": "viewer", "want": "screen", "require_attended_view": true}); err != nil {
		t.Fatal(err)
	}
	frame, err := client.readOne()
	if err != nil {
		t.Fatal(err)
	}
	_ = frame
	host := <-done
	if host != nil {
		host.Close()
		t.Fatal("attended-view request silently admitted by legacy headless listener")
	}
	select {
	case <-called:
		t.Fatal("headless admission callback ran for attended-only request")
	default:
	}
}

type testAttendedListener struct{ net.Conn }

func (l testAttendedListener) Accept() (net.Conn, error) { return l.Conn, nil }
func (l testAttendedListener) Close() error              { return nil }
func (l testAttendedListener) Addr() net.Addr            { return l.LocalAddr() }

func TestAttendedViewExplicitPathAndRefusal(t *testing.T) {
	for _, tc := range []struct {
		name                string
		attended, allow, ok bool
	}{{"approved-attended", true, true, true}, {"declined-attended", true, false, false}, {"headless", false, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			done := make(chan *Mux, 1)
			go func() {
				listen := Listen
				if tc.attended {
					listen = ListenAttended
				}
				m, _ := listen(testAttendedListener{b}, "host", func(Hello, net.Addr) (bool, string) { return tc.allow, "fixture local answer" })
				done <- m
			}()
			client, err := DialAttendedView(a, "viewer", func(reply HelloReply, _ net.Addr) error {
				if !reply.AttendedView {
					t.Error("attended reply missing")
				}
				return nil
			})
			if (err == nil) != tc.ok {
				t.Fatalf("result %v", err)
			}
			if client != nil {
				client.Close()
			}
			if host := <-done; host != nil {
				host.Close()
			}
		})
	}
}

func TestAttendedViewRejectsLegacyReplyWithoutMarker(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		m := newMux(b, StreamFile+1)
		if _, err := m.readOne(); err != nil {
			return
		}
		_ = m.sendJSON(StreamControl, msgData, PriInteractive, HelloReply{Version: protoVersion, NodeName: "legacy-host", Accepted: true})
	}()
	client, err := DialAttendedView(a, "viewer", nil)
	if client != nil {
		client.Close()
	}
	if err == nil {
		t.Fatal("legacy reply silently accepted")
	}
	<-done
}
