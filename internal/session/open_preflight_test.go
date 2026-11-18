package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOpenPreflightRefusesOversizeBeforeOwnership(t *testing.T) {
	for _, started := range []bool{false, true} {
		m, _ := ownedRecvFixture(t)
		m.started = started
		m.idMu.Lock()
		beforeID := m.nextID
		m.idMu.Unlock()
		m.streamMu.Lock()
		beforeStreams := len(m.streams)
		m.streamMu.Unlock()
		m.wmu.Lock()
		beforeOpened := len(m.opened)
		m.wmu.Unlock()
		_, err := m.Open(PriBulk, strings.Repeat("x", maxOpenPayload))
		if err == nil || !strings.Contains(err.Error(), "OPEN metadata") {
			t.Fatal("oversized local OPEN not refused", started, err)
		}
		m.idMu.Lock()
		afterID := m.nextID
		m.idMu.Unlock()
		m.streamMu.Lock()
		afterStreams := len(m.streams)
		m.streamMu.Unlock()
		m.wmu.Lock()
		queued, afterOpened := m.queuedBytes, len(m.opened)
		m.wmu.Unlock()
		if afterID != beforeID || afterStreams != beforeStreams || queued != 0 || afterOpened != beforeOpened {
			t.Fatal("rejected OPEN took ownership", afterID, beforeID, afterStreams, beforeStreams, queued, afterOpened, beforeOpened)
		}
		select {
		case <-m.Closed():
			t.Fatal("local preflight ended unrelated mux")
		default:
		}
	}
}

func TestOpenPreflightRejectsWithoutEndingLiveSession(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	incoming := make(chan Stream, 2)
	host.OnStream(func(s Stream) { incoming <- s })
	if _, err := client.Open(PriBulk, strings.Repeat("x", maxOpenPayload)); err == nil {
		t.Fatal("oversized live OPEN accepted")
	}
	// Exactly16KiB serialized metadata is still valid under the existing limit.
	raw := json.RawMessage(`"` + strings.Repeat("x", maxOpenPayload-2) + `"`)
	sent, err := client.Open(PriBulk, raw)
	if err != nil {
		t.Fatal("valid boundary refused", err)
	}
	defer sent.release()
	select {
	case s := <-incoming:
		defer s.release()
		if len(s.OpenJSON) != maxOpenPayload {
			t.Fatal("boundary metadata changed", len(s.OpenJSON))
		}
		if err := host.SendJSON(s.ID, PriBulk, FileAccept{OK: true}); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("valid Open lost after local rejection")
	}
	frame, err := sent.Recv(time.Second)
	if err != nil {
		t.Fatal("unrelated stream no longer usable", err)
	}
	var accept FileAccept
	if err := json.Unmarshal(frame.Payload, &accept); err != nil || !accept.OK {
		t.Fatal("valid reply lost", err)
	}
	select {
	case <-client.Closed():
		t.Fatal("rejected local Open closed active session")
	default:
	}
}
