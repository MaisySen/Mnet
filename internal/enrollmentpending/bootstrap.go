package enrollmentpending

// Experimental durable challenge request DATA only. No durable key use, proof
// submission, node/receipt consumption, authority grant or finalization exists.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"time"
	"unicode/utf8"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
)

type bootstrapData struct {
	Binding       contract.Binding `json:"binding"`
	ChallengeJSON string           `json:"challenge_json"`
	StartedMS     int64            `json:"started_ms"`
	TimeoutMS     int64            `json:"timeout_ms"`
	MaxLifetimeMS int64            `json:"max_lifetime_ms"`
}

func (bootstrapData) String() string   { return "[private pending bootstrap DATA]" }
func (bootstrapData) GoString() string { return "[private pending bootstrap DATA]" }
func decodeBootstrap(r record) (bootstrapData, bool) {
	var b bootstrapData
	raw := []byte(r.BootstrapJSON)
	if len(raw) == 0 || len(raw) > 32768 || !bootstrapShape(raw, &b) {
		return b, false
	}
	return b, true
}

// Same recursive exact grammar as journal, never trusted serde/authentication.
func bootstrapShape(raw []byte, out *bootstrapData) bool { return strictBootstrapJSON(raw, out) }
func bindingMatchesOwn(r record, b contract.Binding) bool {
	own, err := contract.DecodeSourceExchange([]byte(r.ExchangeJSON), r.Launch, r.Authority, r.Allowed, r.Allowed, time.UnixMilli(r.CreatedMS))
	if err != nil {
		return false
	}
	descriptor, err := contract.SourceDescriptorDigest(own, r.Policy.ServiceID, b.ReserveID)
	if err != nil {
		return false
	}
	p, m := own.Provenance, own.Mapping
	if b.PrincipalAuthority != p.PrincipalAuthority || b.PrincipalNamespace != p.PrincipalNamespace || b.PrincipalID != p.PrincipalID || b.SessionBinding != p.SessionBinding || b.IssuerLabel != m.Issuer || b.RegistrarOrigin != m.RegistrarOrigin || b.Audience != m.Audience || b.ServiceID != r.Policy.ServiceID || b.WorkspaceID != m.WorkspaceID || b.NetworkID != m.NetworkID || b.MappingRevision != m.MappingRevision || b.HandoffID != own.ID || b.AttemptID != r.Launch.AttemptID || b.DescriptorDigest != descriptor || b.DescriptorEncodingVersion != 1 || b.DevicePublicKey != r.Launch.DevicePublicKey || b.CredentialDigest != r.Launch.CredentialDigest {
		return false
	}
	probe := contract.Challenge{Binding: b, ID: b.AttemptID, Nonce: b.SessionBinding, EphemeralPublicKey: b.DevicePublicKey, ExpiresAtMS: b.AccessUntilMS}
	_, err = contract.Encode(probe)
	return err == nil
}
func validBootstrapRecord(r record, policy *config.LocalPolicyV2) bool {
	if r.Version != 3 || !token(r.Code) || !bindingMatchesOwn(r, bootstrapBinding(r)) {
		return false
	}
	own, err := contract.DecodeSourceExchange([]byte(r.ExchangeJSON), r.Launch, r.Authority, r.Allowed, r.Allowed, time.UnixMilli(r.CreatedMS))
	if err != nil || !policyAllowsExchange(r, policy, own) {
		return false
	}
	b, ok := decodeBootstrap(r)
	if !ok || b.StartedMS < r.CreatedMS || b.StartedMS >= r.ExpiresMS || b.TimeoutMS < 1 || b.TimeoutMS > 60000 || b.MaxLifetimeMS < 1 || b.MaxLifetimeMS > 900000 || b.StartedMS < b.Binding.CheckedAtMS || b.StartedMS >= b.Binding.AccessUntilMS {
		return false
	}
	switch r.Phase {
	case "bootstrap-may-have-started":
		return b.ChallengeJSON == ""
	case "bootstrap-challenge-data":
		c, err := contract.DecodeChallengeWire([]byte(b.ChallengeJSON))
		return err == nil && c.Binding == b.Binding && c.ExpiresAtMS > b.StartedMS && c.ExpiresAtMS-b.Binding.CheckedAtMS <= b.MaxLifetimeMS
	}
	return false
}
func bootstrapBinding(r record) contract.Binding { b, _ := decodeBootstrap(r); return b.Binding }

// BootstrapChallengeDATA admits only a SAME saved exchanged-data intent. Binding
// is proposal DATA checked against own saved launch/provenance/descriptor, not an
// authenticated reserve/current lease. No private credential getter is exposed.
func (j *Journal) BootstrapChallengeDATA(ctx context.Context, binding contract.Binding, current func() SourceConfig, clock func() time.Time, limits registrar.Limits) (contract.Challenge, error) {
	if ctx == nil || ctx.Err() != nil || current == nil || clock == nil {
		return contract.Challenge{}, ErrInvalid
	}
	initial := copyConfig(current())
	now := clock()
	j.mu.Lock()
	if err := j.health(); err != nil {
		j.mu.Unlock()
		return contract.Challenge{}, err
	}
	if j.data == nil || j.data.Phase != "exchanged-data" || j.data.Version != 2 || j.cancel != nil || j.reopened || !live(*j.data, now) || !bootstrapCurrent(*j.data, initial, now) || !bindingMatchesOwn(*j.data, binding) {
		j.mu.Unlock()
		return contract.Challenge{}, ErrInvalid
	}
	// Validate constructor/bounds before marking; no connection is opened here.
	client, err := registrar.New(binding.RegistrarOrigin, binding, j.data.Credential, limits)
	if err != nil {
		j.mu.Unlock()
		return contract.Challenge{}, ErrInvalid
	}
	defer client.Close()
	b := bootstrapData{Binding: binding, StartedMS: now.UnixMilli(), TimeoutMS: limits.Timeout.Milliseconds(), MaxLifetimeMS: limits.MaxChallengeLifetime.Milliseconds()}
	raw, _ := json.Marshal(b)
	r := *j.data
	r.Version = 3
	r.Phase = "bootstrap-may-have-started"
	r.BootstrapJSON = string(raw)
	if err := j.commit(r); err != nil {
		j.mu.Unlock()
		return contract.Challenge{}, err
	}
	requestCtx, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	j.mu.Unlock()
	started := time.Now()
	challenge, sendErr := client.Challenge(requestCtx)
	afterConfig := copyConfig(current())
	after := clock()
	observedAfter := after
	backward := after.Before(now)
	bound := now.Add(time.Since(started))
	if after.Before(bound) {
		after = bound
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	cancel()
	j.cancel = nil
	if err := j.health(); err != nil {
		return contract.Challenge{}, errors.Join(ErrRemoteUnknown, err)
	}
	if sendErr != nil || ctx.Err() != nil || backward || !live(r, after) || !bootstrapCurrent(r, afterConfig, after) || j.data == nil || j.data.Phase != "bootstrap-may-have-started" || challenge.Binding != binding || challenge.ExpiresAtMS <= after.UnixMilli() {
		return contract.Challenge{}, ErrRemoteUnknown
	}
	challengeRaw, _ := json.Marshal(contract.ChallengeWire{Version: 4, Challenge: challenge})
	b.ChallengeJSON = string(challengeRaw)
	raw, _ = json.Marshal(b)
	r.Phase = "bootstrap-challenge-data"
	r.BootstrapJSON = string(raw)
	if err := j.commit(r); err != nil {
		return contract.Challenge{}, errors.Join(ErrRemoteUnknown, err)
	}
	// Historical durable DATA remains if current checks change after publication.
	j.mu.Unlock()
	finalConfig := copyConfig(current())
	finalNow := clock()
	j.mu.Lock()
	if err := j.health(); err != nil {
		return contract.Challenge{}, errors.Join(ErrNoLongerCurrent, err)
	}
	finalBackward := finalNow.Before(observedAfter)
	finalBound := now.Add(time.Since(started))
	if finalNow.Before(finalBound) {
		finalNow = finalBound
	}
	if finalBackward || !live(r, finalNow) || !bootstrapCurrent(r, finalConfig, finalNow) || challenge.ExpiresAtMS <= finalNow.UnixMilli() {
		return contract.Challenge{}, ErrNoLongerCurrent
	}
	return challenge, nil
}

func strictBootstrapJSON(raw []byte, out *bootstrapData) bool {
	if !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !shape(d, reflect.TypeOf(*out), 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func bootstrapCurrent(r record, c SourceConfig, now time.Time) bool {
	if !configMatches(r, c) {
		return false
	}
	own, err := contract.DecodeSourceExchange([]byte(r.ExchangeJSON), r.Launch, r.Authority, r.Allowed, c.Allowed, now)
	return err == nil && policyAllowsExchange(r, c.Policy, own)
}
