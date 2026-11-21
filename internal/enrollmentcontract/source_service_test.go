package enrollmentcontract

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestServiceReserveParserUsesActualSentRequestNotSyntheticNativeIdentity(t *testing.T) {
	own, reserve, _, raw, now := sourceFixture(t)
	sent := SourceReserveRequest{Ticket: own.Ticket, HandoffID: own.ID, ReserveID: reserve.ReserveID, Launch: own.Launch, Mapping: own.Mapping}
	parsed, err := DecodeServiceReserve(raw, sent, own.Provenance.PrincipalAuthority, reserve.ServiceID, now)
	if err != nil || parsed != reserve {
		t.Fatal("actual producer service response rejected", err)
	}
	descriptor, err := ServiceReserveDescriptorDigest(sent, reserve.ServiceID)
	if err != nil || descriptor != reserve.DescriptorDigest {
		t.Fatal("request-based descriptor parity failed", err)
	}
	nativeBytes, err := SourceDescriptorBytes(own, reserve.ServiceID, reserve.ReserveID)
	if err != nil {
		t.Fatal(err)
	}
	serviceBytes, err := ServiceReserveDescriptorBytes(sent, reserve.ServiceID)
	if err != nil || !bytes.Equal(nativeBytes, serviceBytes) {
		t.Fatal("same actual tuple has different descriptor encodings", err)
	}
	// Registrar cannot derive expected native principal from this response. Source
	// controls the observed row principal; the DATA parser must not pretend it had
	// an independently authenticated native exchange result to compare against.
	changed := reserve
	changed.Provenance.PrincipalID = "different-source-row-principal"
	different, _ := json.Marshal(changed)
	if observed, err := DecodeServiceReserve(different, sent, own.Provenance.PrincipalAuthority, reserve.ServiceID, now); err != nil || observed.Provenance.PrincipalID != changed.Provenance.PrincipalID {
		t.Fatal("service DATA parser invented native expected principal", err)
	}
	if _, err := DecodeSourceReserve(different, own, reserve.ServiceID, reserve.ReserveID, reserve.DescriptorDigest, own.Mapping, now); err == nil {
		t.Fatal("native parser failed independent own-PKCE identity comparison")
	}
	for _, bad := range []SourceReserveRequest{
		{Ticket: "bad", HandoffID: sent.HandoffID, ReserveID: sent.ReserveID, Launch: sent.Launch, Mapping: sent.Mapping},
		{Ticket: sent.Ticket, HandoffID: own.Launch.AttemptID, ReserveID: sent.ReserveID, Launch: sent.Launch, Mapping: sent.Mapping},
		{Ticket: sent.Ticket, HandoffID: sent.HandoffID, ReserveID: own.Launch.AttemptID, Launch: sent.Launch, Mapping: sent.Mapping},
	} {
		if _, err := DecodeServiceReserve(raw, bad, own.Provenance.PrincipalAuthority, reserve.ServiceID, now); err == nil {
			t.Fatal("mismatched sent request accepted")
		}
	}
	if _, err := DecodeServiceReserve(raw, sent, "https://wrong-source-authority.test", reserve.ServiceID, now); err == nil {
		t.Fatal("configured authority not pinned")
	}
}
func TestServiceReserveRequiredNestedFieldsAndNoAuthorityFlags(t *testing.T) {
	own, r, _, raw, now := sourceFixture(t)
	sent := SourceReserveRequest{Ticket: own.Ticket, HandoffID: own.ID, ReserveID: r.ReserveID, Launch: own.Launch, Mapping: own.Mapping}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	check := func(b []byte) bool {
		_, err := DecodeServiceReserve(b, sent, own.Provenance.PrincipalAuthority, r.ServiceID, now)
		return err != nil
	}
	var walk func(map[string]any)
	walk = func(object map[string]any) {
		for key, value := range object {
			delete(object, key)
			b, _ := json.Marshal(root)
			if !check(b) {
				t.Fatal("missing service field accepted")
			}
			object[key] = nil
			b, _ = json.Marshal(root)
			if !check(b) {
				t.Fatal("null service field accepted")
			}
			object[key] = value
			if nested, ok := value.(map[string]any); ok {
				walk(nested)
			}
		}
		object["unexpected"] = false
		b, _ := json.Marshal(root)
		if !check(b) {
			t.Fatal("unknown service field accepted")
		}
		delete(object, "unexpected")
	}
	walk(root)
	for _, key := range []string{"authorityGranted", "enrolled"} {
		root[key] = true
		b, _ := json.Marshal(root)
		if !check(b) {
			t.Fatal("service response claimed authority accepted")
		}
		root[key] = false
	}
}
