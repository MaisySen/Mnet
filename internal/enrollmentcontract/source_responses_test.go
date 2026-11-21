package enrollmentcontract

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func sourceFixture(t *testing.T) (SourceExchange, SourceReserve, []byte, []byte, time.Time) {
	t.Helper()
	raw, err := os.ReadFile("testdata/multies-provenance-source-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FixtureOnly bool           `json:"fixtureOnly"`
		Exchange    SourceExchange `json:"exchange"`
		Reserve     SourceReserve  `json:"reserve"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || !fixture.FixtureOnly {
		t.Fatal("invalid disposable source fixture", err)
	}
	exchange, _ := json.Marshal(fixture.Exchange)
	reserve, _ := json.Marshal(fixture.Reserve)
	now, ok := sourceDate(fixture.Reserve.CheckedAt)
	if !ok {
		t.Fatal("fixture checkedAt invalid")
	}
	return fixture.Exchange, fixture.Reserve, exchange, reserve, now
}
func TestSourceResponsesMatchActualMultiesProducerFixture(t *testing.T) {
	own, r, exchange, reserve, now := sourceFixture(t)
	parsed, err := DecodeSourceExchange(exchange, own.Launch, own.Provenance.PrincipalAuthority, []SourceMapping{own.Mapping}, []SourceMapping{own.Mapping}, now)
	if err != nil || parsed != own {
		t.Fatal("actual own-PKCE response rejected", err)
	}
	parsedReserve, err := DecodeSourceReserve(reserve, parsed, r.ServiceID, r.ReserveID, r.DescriptorDigest, own.Mapping, now)
	if err != nil || parsedReserve != r {
		t.Fatal("actual service-source response rejected", err)
	}
	// Exact config must still be in BOTH original and current independent snapshots.
	for _, saved := range [][]SourceMapping{nil, {{WorkspaceID: "caller-invented"}}} {
		if _, err := DecodeSourceExchange(exchange, own.Launch, own.Provenance.PrincipalAuthority, saved, []SourceMapping{own.Mapping}, now); err == nil {
			t.Fatal("missing original allowed mapping accepted")
		}
	}
	if _, err := DecodeSourceExchange(exchange, own.Launch, own.Provenance.PrincipalAuthority, []SourceMapping{own.Mapping}, nil, now); err == nil {
		t.Fatal("removed current config accepted")
	}
	changed := own.Launch
	changed.CredentialDigest = own.Launch.AttemptID
	if _, err := DecodeSourceExchange(exchange, changed, own.Provenance.PrincipalAuthority, []SourceMapping{own.Mapping}, []SourceMapping{own.Mapping}, now); err == nil {
		t.Fatal("changed presaved native digest accepted")
	}
	if _, err := DecodeSourceExchange(exchange, own.Launch, "https://untrusted.test", []SourceMapping{own.Mapping}, []SourceMapping{own.Mapping}, now); err == nil {
		t.Fatal("response selected its own trusted authority")
	}
	if _, err := DecodeSourceReserve(reserve, own, r.ServiceID, r.ReserveID, own.Launch.AttemptID, own.Mapping, now); err == nil {
		t.Fatal("wrong independently selected descriptor accepted")
	}
	deadline, _ := sourceDate(r.Deadline)
	if _, err := DecodeSourceReserve(reserve, own, r.ServiceID, r.ReserveID, r.DescriptorDigest, own.Mapping, deadline); err == nil {
		t.Fatal("deadline equality accepted")
	}
}
func TestSourceResponseStrictFieldsAliasesFlagsAndDates(t *testing.T) {
	own, r, exchange, reserve, now := sourceFixture(t)
	mutate := func(raw []byte, key string, value any) []byte {
		t.Helper()
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		data[key] = value
		b, _ := json.Marshal(data)
		return b
	}
	for _, raw := range [][]byte{
		append(exchange, []byte(` {}`)...),
		bytes.Replace(exchange, []byte(`"responseVersion":2`), []byte(`"responseVersion":2,"responseVersion":2`), 1),
		bytes.Replace(exchange, []byte(`"responseVersion"`), []byte(`"ResponseVersion"`), 1),
		mutate(exchange, "enrolled", true), mutate(exchange, "inputAuthorized", true), mutate(exchange, "registrarConnected", true),
		mutate(exchange, "sessionId", "raw-session"), mutate(exchange, "provenance", nil), mutate(exchange, "responseVersion", 1),
		mutate(exchange, "expiresAt", "2099-01-01T00:00:00Z"), mutate(exchange, "expiresAt", "2099-01-01T00:00:00.000+00:00"),
		bytes.Repeat([]byte(" "), 65537), []byte(`null`),
	} {
		if _, err := DecodeSourceExchange(raw, own.Launch, own.Provenance.PrincipalAuthority, []SourceMapping{own.Mapping}, []SourceMapping{own.Mapping}, now); err == nil {
			t.Fatal("malformed/aliased/future authority response accepted")
		}
	}
	for _, raw := range [][]byte{
		mutate(reserve, "authorityGranted", true), mutate(reserve, "status", "consumed"), mutate(reserve, "accessUntil", "future"),
		mutate(reserve, "serviceId", "attacker"), mutate(reserve, "reserveId", own.Launch.AttemptID), mutate(reserve, "checkedAt", now.Add(time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")),
		mutate(reserve, "descriptorEncodingVersion", 2), mutate(reserve, "binding", nil),
		bytes.Replace(reserve, []byte(`"sessionBinding"`), []byte(`"sessionId"`), 1),
	} {
		if _, err := DecodeSourceReserve(raw, own, r.ServiceID, r.ReserveID, r.DescriptorDigest, own.Mapping, now); err == nil {
			t.Fatal("changed/expired/unexpected service evidence accepted")
		}
	}
}
func FuzzSourceResponseDecoderNeverPanics(f *testing.F) {
	raw, err := os.ReadFile("testdata/multies-provenance-source-v2.json")
	if err != nil {
		f.Fatal(err)
	}
	var fixture struct {
		Exchange SourceExchange `json:"exchange"`
		Reserve  SourceReserve  `json:"reserve"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		f.Fatal(err)
	}
	own, r := fixture.Exchange, fixture.Reserve
	now, ok := sourceDate(r.CheckedAt)
	if !ok {
		f.Fatal("bad fixture time")
	}
	exchange, _ := json.Marshal(own)
	reserve, _ := json.Marshal(r)
	f.Add(exchange)
	f.Add(reserve)
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 70000 {
			t.Skip()
		}
		DecodeSourceExchange(raw, own.Launch, own.Provenance.PrincipalAuthority, []SourceMapping{own.Mapping}, []SourceMapping{own.Mapping}, now)
		DecodeSourceReserve(raw, own, r.ServiceID, r.ReserveID, r.DescriptorDigest, own.Mapping, now)
	})
}

// Every nested field is required and null is never a zero-value substitute.
func TestSourceResponseEveryFieldRequiredNonNullAndExact(t *testing.T) {
	own, r, exchange, reserve, now := sourceFixture(t)
	for name, raw := range map[string][]byte{"exchange": exchange, "reserve": reserve} {
		check := func(b []byte) bool {
			if name == "exchange" {
				_, err := DecodeSourceExchange(b, own.Launch, own.Provenance.PrincipalAuthority, []SourceMapping{own.Mapping}, []SourceMapping{own.Mapping}, now)
				return err != nil
			}
			_, err := DecodeSourceReserve(b, own, r.ServiceID, r.ReserveID, r.DescriptorDigest, own.Mapping, now)
			return err != nil
		}
		var root map[string]any
		if err := json.Unmarshal(raw, &root); err != nil {
			t.Fatal(err)
		}
		var walk func(map[string]any, string)
		walk = func(object map[string]any, path string) {
			for key, value := range object {
				delete(object, key)
				b, _ := json.Marshal(root)
				if !check(b) {
					t.Fatalf("%s missing %s.%s accepted", name, path, key)
				}
				object[key] = nil
				b, _ = json.Marshal(root)
				if !check(b) {
					t.Fatalf("%s null %s.%s accepted", name, path, key)
				}
				object[key] = value
				if nested, ok := value.(map[string]any); ok {
					walk(nested, path+"."+key)
				}
			}
			object["unknownExtra"] = false
			b, _ := json.Marshal(root)
			if !check(b) {
				t.Fatalf("%s unknown at %s accepted", name, path)
			}
			delete(object, "unknownExtra")
		}
		walk(root, "root")
	}
}
func TestSourceNestedDuplicateAliasesAndUnicode(t *testing.T) {
	own, _, exchange, _, now := sourceFixture(t)
	for _, raw := range [][]byte{
		bytes.Replace(exchange, []byte(`"principalNamespace":`), []byte(`"principalId":"other","principalNamespace":`), 1),
		bytes.Replace(exchange, []byte(`"principalId":`), []byte(`"PrincipalId":`), 1),
		bytes.Replace(exchange, []byte(`"principalId":`), []byte(`"principal\u0049d":"other","principalId":`), 1),
		bytes.Replace(exchange, []byte(`"principalId":`), []byte(`"principalId":"\ud800","unused":`), 1),
		bytes.Replace(exchange, []byte(`"enrolled":false`), []byte(`"enrolled":0`), 1),
		bytes.Replace(exchange, []byte(`"responseVersion":2`), []byte(`"responseVersion":2.0`), 1),
		append([]byte{0xff}, exchange...),
	} {
		if _, err := DecodeSourceExchange(raw, own.Launch, own.Provenance.PrincipalAuthority, []SourceMapping{own.Mapping}, []SourceMapping{own.Mapping}, now); err == nil {
			t.Fatal("nested alias/duplicate/invalid scalar accepted")
		}
	}
}
