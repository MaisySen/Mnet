package enrollmentpending

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	source "fintex.au/mnet/internal/enrollmentsource"
)

func changedPolicy(t *testing.T, c SourceConfig, change func(*config.LocalConfigV2)) SourceConfig {
	t.Helper()
	snapshot, ok := c.Policy.Snapshot()
	if !ok {
		t.Fatal("missing policy")
	}
	change(&snapshot)
	p, err := config.ValidateLocalV2(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	out, ok := SourceConfigFor(p)
	if !ok {
		t.Fatal("bad changed policy")
	}
	return out
}
func TestPolicyScopePreflightRefusesZeroSends(t *testing.T) {
	for _, kind := range []string{"principal-removed", "service-change", "source-change", "namespace-missing", "mapping-change", "mixed-fields", "nil"} {
		t.Run(kind, func(t *testing.T) {
			j, now, c := callbackReady(t)
			calls := 0
			j.send = func(context.Context, record, SourceConfig, source.Limits, time.Time) (contract.SourceExchange, error) {
				calls++
				return contract.SourceExchange{}, nil
			}
			switch kind {
			case "principal-removed":
				c = changedPolicy(t, c, func(p *config.LocalConfigV2) { p.Bindings[0].PrincipalIDs = []string{"other"} })
			case "service-change":
				c = changedPolicy(t, c, func(p *config.LocalConfigV2) { p.ServiceID = "another-service" })
			case "source-change":
				c = changedPolicy(t, c, func(p *config.LocalConfigV2) { p.SourceOrigin = "https://other.test" })
			case "mapping-change":
				c = changedPolicy(t, c, func(p *config.LocalConfigV2) { p.Revision = "next" })
			case "namespace-missing":
				c.Policy = &config.LocalPolicyV2{}
			case "mixed-fields":
				c.Allowed[0].NetworkID = "mixed"
			case "nil":
				c.Policy = nil
			}
			if _, err := j.ExchangeSaved(context.Background(), func() SourceConfig { return c }, func() time.Time { return now }, sourceLimits()); !errors.Is(err, ErrInvalid) || calls != 0 || j.data.Phase != "callback-recorded" {
				t.Fatal("invalid policy admitted private send", err, calls)
			}
		})
	}
}
func TestResponseCannotExpandOriginalOrCurrentPrincipalScope(t *testing.T) {
	for _, kind := range []string{"response-added-principal", "current-revoked-principal", "postsave-revocation", "service-changed-inflight", "postsave-service"} {
		t.Run(kind, func(t *testing.T) {
			j, now, c := callbackReady(t)
			var result contract.SourceExchange
			json.Unmarshal(exchange(t, j, now), &result)
			calls := 0
			reads := 0
			if kind == "response-added-principal" {
				result.Provenance.PrincipalID = "new-from-response"
				c = changedPolicy(t, c, func(p *config.LocalConfigV2) {
					p.Bindings[0].PrincipalIDs = append(p.Bindings[0].PrincipalIDs, "new-from-response")
				})
			}
			j.send = func(context.Context, record, SourceConfig, source.Limits, time.Time) (contract.SourceExchange, error) {
				calls++
				return result, nil
			}
			current := func() SourceConfig {
				reads++
				if reads >= 2 && kind == "current-revoked-principal" || reads >= 3 && kind == "postsave-revocation" {
					return changedPolicy(t, c, func(p *config.LocalConfigV2) { p.Bindings[0].PrincipalIDs = []string{"other"} })
				}
				if reads >= 2 && kind == "service-changed-inflight" || reads >= 3 && kind == "postsave-service" {
					return changedPolicy(t, c, func(p *config.LocalConfigV2) { p.ServiceID = "changed-service" })
				}
				return c
			}
			got, err := j.ExchangeSaved(context.Background(), current, func() time.Time { return now }, sourceLimits())
			expected := ErrRemoteUnknown
			phase := "exchange-may-have-started"
			if kind == "postsave-revocation" || kind == "postsave-service" {
				expected = ErrNoLongerCurrent
				phase = "exchanged-data"
			}
			if !errors.Is(err, expected) || got != (contract.SourceExchange{}) || calls != 1 || j.data.Phase != phase {
				t.Fatal("policy change/response expanded authority", err, calls, j.data.Phase)
			}
		})
	}
}
func TestPolicySnapshotPersistedWithoutCallerAlias(t *testing.T) {
	j, _, _, dir := prepared(t)
	original := j.data.Policy.ServiceID
	c := savedConfig(t, j)
	snapshot, _ := c.Policy.Snapshot()
	snapshot.ServiceID = "changed"
	snapshot.Bindings[0].PrincipalIDs[0] = "changed"
	raw, err := os.ReadFile(j.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"policy":`)) {
		t.Fatal("policy not persisted")
	}
	j.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.data.Version != 2 || reopened.data.Policy.ServiceID != original || reopened.data.Policy.Bindings[0].PrincipalIDs[0] != "fixture-local-user" {
		t.Fatal("snapshot mutated")
	}
}
func TestLegacyIncompletePolicyJournalRefusesInPlaceWithoutReset(t *testing.T) {
	j, _, _, dir := prepared(t)
	path := j.path
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	j.Close()
	var record map[string]json.RawMessage
	json.Unmarshal(raw, &record)
	delete(record, "policy")
	record["version"] = json.RawMessage(`1`)
	legacy, _ := json.Marshal(record)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(dir); err == nil {
		reopened.Close()
		t.Fatal("legacy intent silently migrated")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, legacy) {
		t.Fatal("legacy intent changed")
	}
	if _, err := os.Stat(filepath.Join(dir, "enrollment-pending-v2.json")); !os.IsNotExist(err) {
		t.Fatal("parallel fresh file created")
	}
}
func TestRecordExchangeCannotBypassSavedPolicy(t *testing.T) {
	j, l, now, _ := prepared(t)
	j.RecordCallback(l.AttemptID, l.State, code(), now)
	j.MarkExchangeStarted(now)
	c := savedConfig(t, j)
	var data contract.SourceExchange
	json.Unmarshal(exchange(t, j, now), &data)
	data.Provenance.PrincipalID = "other"
	raw, _ := json.Marshal(data)
	c = changedPolicy(t, c, func(p *config.LocalConfigV2) {
		p.Bindings[0].PrincipalIDs = append(p.Bindings[0].PrincipalIDs, "other")
	})
	if err := j.RecordExchange(raw, c, now); !errors.Is(err, ErrInvalid) || j.data.Phase != "exchange-may-have-started" {
		t.Fatal("DATA setter expanded saved policy", err)
	}
}
