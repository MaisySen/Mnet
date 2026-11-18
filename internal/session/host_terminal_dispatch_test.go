package session

import (
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostTerminalDuringAuthorizationRefusesDispatch(t *testing.T) {
	for _, kind := range []string{"input", "chat", "capture"} {
		for _, end := range []string{"mux", "stop", "denial"} {
			t.Run(kind+"/"+end, func(t *testing.T) {
				m, s := ownedRecvFixture(t)
				entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var chats atomic.Int32
				cap := &authorizationCapturer{}
				inj := &authorizationInjector{}
				h := &Host{Mux: m, Cap: cap, Inj: inj, FPS: 240, stop: make(chan struct{}), onChat: func(ChatMessage) { chats.Add(1) }}
				h.authorize = func() bool { close(entered); <-release; return true }
				switch kind {
				case "input":
					b, _ := json.Marshal(InputEvent{Kind: "pointer", Action: "click"})
					s.in <- Frame{Type: msgData, Payload: b}
				case "chat":
					b, _ := json.Marshal(ChatMessage{Text: "fixture"})
					s.in <- Frame{Type: msgData, Payload: b}
				}
				h.wg.Add(1)
				go func() {
					defer close(done)
					switch kind {
					case "input":
						h.inputLoop(s)
					case "chat":
						h.chatLoop(s)
					case "capture":
						h.screenLoop()
					}
				}()
				select {
				case <-entered:
				case <-time.After(time.Second):
					close(release)
					t.Fatal("authorization not entered")
				}
				switch end {
				case "mux":
					m.Close()
				case "stop":
					close(h.stop)
				case "denial":
					h.revoked.Store(true)
				}
				close(release)
				select {
				case <-done:
				case <-time.After(time.Second):
					m.Close()
					t.Fatal("late authorization retained worker")
				}
				h.wg.Wait()
				if inj.calls.Load() != 0 || chats.Load() != 0 || cap.captures.Load() != 0 {
					t.Fatal("late callback revived dispatch", inj.calls.Load(), chats.Load(), cap.captures.Load())
				}
				if h.allowed() {
					t.Fatal("terminal host revived")
				}
			})
		}
	}
}

func TestHostTerminalSkipsNewAuthorization(t *testing.T) {
	for _, end := range []string{"mux", "stop", "denial"} {
		m, _ := ownedRecvFixture(t)
		h := &Host{Mux: m, stop: make(chan struct{})}
		var calls atomic.Int32
		h.authorize = func() bool { calls.Add(1); return true }
		switch end {
		case "mux":
			m.Close()
		case "stop":
			close(h.stop)
		case "denial":
			h.revoked.Store(true)
		}
		if h.allowed() || calls.Load() != 0 {
			t.Fatal("terminal host dispatched authorization", end, calls.Load())
		}
	}
}
