//go:build mnet_enrollment_development

package osownership

// Both the inert protocol and a future real adapter share this private ledger
// identity contract. It exposes no effects and supplies no execution release.
// The implementing adapter must own this exact stable session from creation to
// teardown; a per-call manufactured session can never match original custody.
type transactionLedgerAdapter interface {
	Kernel
	ownershipSession() *adapterSession
}

func (a *inertAdapterProtocol) ownershipSession() *adapterSession {
	if a == nil {
		return nil
	}
	return a.session
}
