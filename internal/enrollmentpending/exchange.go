package enrollmentpending

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	source "fintex.au/mnet/internal/enrollmentsource"
)

var ErrNoLongerCurrent = errors.New("enrollment exchange DATA was saved but is no longer current")

var ErrRemoteUnknown = errors.New("enrollment exchange result not durably recorded; remote outcome may be unknown")

// SourceConfig is caller DATA from the future installed config owner, not an
// operator grant or a validated native State. Source origin/authority are distinct.
type SourceConfig struct {
	SourceOrigin, PrincipalAuthority string
	Allowed                          []contract.SourceMapping
}
type exchangeSender func(context.Context, record, SourceConfig, source.Limits, time.Time) (contract.SourceExchange, error)

func sendNativeExchange(ctx context.Context, r record, c SourceConfig, limits source.Limits, now time.Time) (contract.SourceExchange, error) {
	n, err := source.NewNative(c.SourceOrigin, c.PrincipalAuthority, limits)
	if err != nil {
		return contract.SourceExchange{}, err
	}
	defer n.CloseIdleConnections()
	return n.Exchange(ctx, r.Launch, r.SourceOrigin, r.Authority, r.Code, r.Verifier, r.Allowed, c.Allowed, now)
}
func copyConfig(c SourceConfig) SourceConfig {
	c.Allowed = append([]contract.SourceMapping(nil), c.Allowed...)
	return c
}
func configMatches(r record, c SourceConfig) bool {
	if c.SourceOrigin != r.SourceOrigin || c.PrincipalAuthority != r.Authority || len(c.Allowed) == 0 || len(c.Allowed) > 20 {
		return false
	}
	intersection := false
	for _, m := range c.Allowed {
		if _, err := contract.SourceDescriptorBytes(contract.SourceExchange{ID: r.Launch.AttemptID, Launch: r.Launch, Mapping: m}, "native-config-shape", r.Launch.AttemptID); err != nil {
			return false
		}
		for _, original := range r.Allowed {
			if m == original {
				intersection = true
			}
		}
	}
	return intersection
}

// ExchangeSaved is disconnected journal/client integration, not a native key,
// listener, config owner or authority permit. The future daemon must provide
// actual current installed config and trusted time under its own lifecycle.
// No private credential/verifier getter is introduced. Only callback-recorded
// can be admitted, once. Local may-have-started publication precedes network I/O.
func (j *Journal) ExchangeSaved(ctx context.Context, current func() SourceConfig, clock func() time.Time, limits source.Limits) (contract.SourceExchange, error) {
	if ctx == nil || ctx.Err() != nil || current == nil || clock == nil {
		return contract.SourceExchange{}, ErrInvalid
	}
	c := copyConfig(current())
	now := clock()
	j.mu.Lock()
	if err := j.health(); err != nil {
		j.mu.Unlock()
		return contract.SourceExchange{}, err
	}
	if j.data == nil || j.data.Phase != "callback-recorded" || j.cancel != nil || !live(*j.data, now) || !configMatches(*j.data, c) {
		j.mu.Unlock()
		return contract.SourceExchange{}, ErrInvalid
	}
	// Validate constructor bounds BEFORE marking any possibility of remote work.
	preflight, err := source.NewNative(c.SourceOrigin, c.PrincipalAuthority, limits)
	if err != nil {
		j.mu.Unlock()
		return contract.SourceExchange{}, ErrInvalid
	}
	preflight.CloseIdleConnections()
	r := *j.data
	r.Phase = "exchange-may-have-started"
	if err := j.commit(r); err != nil {
		j.mu.Unlock()
		return contract.SourceExchange{}, err
	}
	requestCtx, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	send := j.send
	j.mu.Unlock()
	started := time.Now()
	data, sendErr := send(requestCtx, r, c, limits, now)
	afterConfig := copyConfig(current())
	after := clock()
	clockAfter := after
	elapsedBound := now.Add(time.Since(started))
	backward := after.Before(now)
	if after.Before(elapsedBound) {
		after = elapsedBound
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	cancel()
	j.cancel = nil
	if err := j.health(); err != nil {
		return contract.SourceExchange{}, errors.Join(ErrRemoteUnknown, err)
	}
	// Failure after successful admission never clears/replaces original intent or
	// exposes a ticket, even if cancellation occurred before a server response.
	if sendErr != nil || ctx.Err() != nil || backward || !live(r, after) || !configMatches(r, afterConfig) {
		return contract.SourceExchange{}, ErrRemoteUnknown
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return contract.SourceExchange{}, ErrRemoteUnknown
	}
	if _, err := contract.DecodeSourceExchange(raw, r.Launch, r.Authority, r.Allowed, afterConfig.Allowed, after); err != nil {
		return contract.SourceExchange{}, ErrRemoteUnknown
	}
	if j.data == nil || j.data.Phase != "exchange-may-have-started" {
		return contract.SourceExchange{}, ErrRemoteUnknown
	}
	r.Phase = "exchanged-data"
	r.ExchangeJSON = string(raw)
	if err := j.commit(r); err != nil {
		return contract.SourceExchange{}, errors.Join(ErrRemoteUnknown, err)
	}
	j.mu.Unlock()
	finalNow := clock()
	j.mu.Lock()
	if err := j.health(); err != nil {
		return contract.SourceExchange{}, errors.Join(ErrNoLongerCurrent, err)
	}
	if finalNow.Before(clockAfter) {
		return contract.SourceExchange{}, ErrNoLongerCurrent
	}
	finalBound := now.Add(time.Since(started))
	if finalNow.Before(finalBound) {
		finalNow = finalBound
	}
	if !live(r, finalNow) {
		return contract.SourceExchange{}, ErrNoLongerCurrent
	}
	if _, err := contract.DecodeSourceExchange(raw, r.Launch, r.Authority, r.Allowed, afterConfig.Allowed, finalNow); err != nil {
		return contract.SourceExchange{}, ErrNoLongerCurrent
	}
	return data, nil
}
