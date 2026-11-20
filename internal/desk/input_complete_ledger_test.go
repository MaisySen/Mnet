package desk

import (
	"errors"
	"testing"
)

func TestRecipientCompletePhysicalLedgerKeepsKeyButtonNamespacesSeparate(t *testing.T) {
	for _, failRelease := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "unconfirmed"}[failRelease], func(t *testing.T) {
			backend := &ownedFixture{}
			sentinel := errors.New("fixture complete ledger release failed")
			if failRelease {
				backend.releaseErr = sentinel
			}
			recipient, err := NewRecipient(backend, 65535, 65535, func() bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			var sequence uint64
			send := func(kind, action string, code uint16) {
				t.Helper()
				sequence++
				if err := recipient.Apply(Input{Sequence: sequence, Kind: kind, Action: action, Code: code}); err != nil {
					t.Fatal(kind, action, code, err)
				}
			}
			for code := uint16(8); code <= 255; code++ {
				send("key", "down", code)
			}
			for code := uint16(1); code <= 5; code++ {
				send("button", "down", code)
			}
			if !recipient.Held() {
				t.Fatal("complete ledger empty")
			}
			for code := uint16(255); code >= 8; code-- {
				send("key", "repeat", code)
			}
			for code := uint16(8); code <= 255; code++ {
				send("key", "up", code)
			}
			if !recipient.Held() {
				t.Fatal("key releases discarded held buttons")
			}
			for code := uint16(5); code >= 2; code-- {
				send("button", "up", code)
			}
			if !recipient.Held() {
				t.Fatal("last held button lost")
			}
			before := len(backend.events)
			for range 3 {
				err := recipient.Close()
				if errors.Is(err, sentinel) != failRelease || (err != nil) != failRelease {
					t.Fatal("cleanup result", err)
				}
				if recipient.Held() != failRelease {
					t.Fatal("ledger differs from cleanup confirmation")
				}
				sequence++
				if err = recipient.Apply(Input{Sequence: sequence, Kind: "button", Action: "up", Code: 1}); err == nil || failRelease && !errors.Is(err, sentinel) {
					t.Fatal("retired last release accepted or uncertainty hidden", err)
				}
			}
			if backend.released != 1 || len(backend.events) != before || before != 248*3+5+4 {
				t.Fatal("native ledger accounting", backend.released, len(backend.events), before)
			}
		})
	}
}
