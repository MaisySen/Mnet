//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
)

// No generic kind allowance. This capability binds one exact first-step token
// and is consumed BEFORE send, even if send returns error. Only first NEWLINK
// update ACK-only5 request equal to that token can enter a sender callback.
type firstLinkSendPermit struct {
	token *packetSetupTransition
	used  bool
}

func (p *firstLinkSendPermit) send(raw []byte, localPID uint32, effect func() error) error {
	if p == nil || p.token == nil || p.token.ledger == nil || effect == nil {
		return ErrConflict
	}
	t := p.token
	l := t.ledger
	l.mu.Lock()
	if p.used || t.owner == nil || t.owner.stopStarted || t.owner.fdCloseAttempted || t.owner.clock == nil || t.owner.deadline.expired(t.owner.clock.Now()) || l.setup != t || t.done || l.current.closed || l.current.generation != t.generation || t.operation.Kind != "set-alias-mtu-exact" || !bytes.Equal(raw, t.request) || len(raw) < 32 || binary.NativeEndian.Uint16(raw[4:]) != 16 || binary.NativeEndian.Uint32(raw[12:]) != localPID {
		l.mu.Unlock()
		return ErrConflict
	}
	flags, e := mutationRequestFlags(16)
	if e != nil || binary.NativeEndian.Uint16(raw[6:]) != flags {
		l.mu.Unlock()
		return ErrConflict
	}
	p.used = true
	// Distinct send-boundary guard accounted into token generation, no delta1.
	if e = l.invalidateLocked("config-intent"); e != nil {
		l.setup = nil
		t.done = true
		l.mu.Unlock()
		return e
	}
	t.generation = l.current.generation
	l.mu.Unlock()
	return effect()
}
