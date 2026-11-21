package daemon

import (
	"context"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	source "fintex.au/mnet/internal/enrollmentsource"
)

// exchangeNativeEnrollment is an inactive private composition, no production
// caller. It can return only source DATA, never a native credential/permit.
// The journal owns one-shot admission and durable remote-unknown outcomes.
func (d *Daemon) exchangeNativeEnrollment(ctx context.Context, limits source.Limits) (contract.SourceExchange, error) {
	d.mu.Lock()
	preparation := d.enrollmentPreparation
	if preparation == nil {
		d.mu.Unlock()
		return contract.SourceExchange{}, errEnrollmentContextStale
	}
	if err := d.checkEnrollmentContextLocked(preparation.context); err != nil {
		d.mu.Unlock()
		return contract.SourceExchange{}, err
	}
	d.mu.Unlock()
	// No native lock is retained across remote I/O. All actual current checks
	// reacquire d.mu, including post-response and post-publication rechecks.
	current := func() pending.SourceConfig {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.enrollmentPreparation != preparation || d.checkEnrollmentContextLocked(preparation.context) != nil {
			return pending.SourceConfig{}
		}
		config, ok := pending.SourceConfigFor(preparation.context.policy)
		if !ok {
			return pending.SourceConfig{}
		}
		return config
	}
	return preparation.journal.ExchangeSaved(ctx, current, time.Now, limits)
}
