//go:build mnet_enrollment_development

package daemon

import "fintex.au/mnet/internal/netmap"

// Session.mu owns this state. Unknown remote outcomes are not acknowledgements.
// The callback is supplied only by the real native session publication paths;
// tests exercise the same state machine with a memory-only failing client.
// This is not a profile/credential/evidence issuer or a production fake selector.
type endpointPublicationState struct{ mayBePublished, closeAttempted bool }

func (p *endpointPublicationState) publish(capability *netmap.EndpointCapability, send func(*netmap.EndpointCapability) error) error {
	if capability != nil {
		p.mayBePublished = true
		p.closeAttempted = false
	}
	err := send(capability)
	if err == nil {
		p.mayBePublished = capability != nil
	}
	return err
}
func (p *endpointPublicationState) withdrawForClose(send func(*netmap.EndpointCapability) error) error {
	if !p.mayBePublished || p.closeAttempted {
		return nil
	}
	p.closeAttempted = true
	err := send(nil)
	if err == nil {
		p.mayBePublished = false
	}
	return err
}
