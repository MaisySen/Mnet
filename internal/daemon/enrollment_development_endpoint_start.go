//go:build mnet_enrollment_development

package daemon

// StartPacketFlow is called by the sole parent lane only AFTER its ADMISSION
// was atomically written. The initial empty-peer bind was stopped before pair
// installation. No peer has run and no transport key/plaintext existed in that
// device generation before this Up. This is a local start boundary, not an
// assertion about when a remote application created its data.
// The caller must still send FLOW_START only after this method succeeds.
func (b *DevelopmentEndpointBridge) StartPacketFlow(p *DevelopmentEndpointPeer) error {
	if b == nil {
		return errPrivateEndpoint
	}
	b.startOnce.Do(func() {
		if _, e := b.Current(p); e != nil {
			b.startErr = e
			return
		}
		s := b.slot
		s.mu.Lock()
		retained := s.dev
		transport := s.pinnedBind
		port := s.announcedPort
		closed := s.closed
		s.mu.Unlock()
		if closed || retained == nil || transport == nil || port == 0 {
			b.startErr = errPrivateEndpoint
			return
		}
		if e := retained.Up(); e != nil {
			b.startErr = e
			return
		}
		if _, e := b.Current(p); e != nil {
			b.startErr = e
			return
		}
		if transport.Port() != port {
			b.startErr = errPrivateEndpoint
			return
		}
		t := s.tun
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.closed || t.active || t.grant != b || b.ctx.Err() != nil {
			b.startErr = errPrivateEndpoint
			return
		}
		// Defensive stale-queue disposal, not the proof that WG in-flight receive
		// work was absent: that proof is the never-running peer/closed bind above.
		for len(t.toPeer) > 0 {
			<-t.toPeer
		}
		t.fromPeer = nil
		if e := t.notify.Drain(); e != nil {
			b.startErr = e
			return
		}
		t.active = true
	})
	if b.startErr != nil {
		_ = b.Close()
		return b.startErr
	}
	_, e := b.Current(p)
	return e
}
