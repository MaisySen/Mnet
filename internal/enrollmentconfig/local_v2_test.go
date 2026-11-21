package enrollmentconfig

import (
	"bytes"
	"encoding/json"
	"testing"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func localFixture() LocalConfigV2 {
	return LocalConfigV2{Version: 2, Revision: "r1", SourceOrigin: "https://source.test", PrincipalAuthority: "https://local-users.test", PrincipalNamespace: contract.PrincipalNamespace, IssuerLabel: "https://issuer-label.test/path", RegistrarOrigin: "https://registrar.test", Audience: "fixture", ServiceID: "service-one", Bindings: []LocalBindingV2{{WorkspaceID: "org_one", NetworkID: "net_one", PrincipalIDs: []string{"local-user-one"}}}}
}
func TestLocalV2DoesNotUpgradeOIDCConfigOrConflateOrigins(t *testing.T) {
	c := localFixture()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseLocalV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err == nil {
		t.Fatal("v1 parser silently accepted local namespace")
	}
	legacy, _ := json.Marshal(fixture())
	if _, err := ParseLocalV2(legacy); err == nil {
		t.Fatal("OIDCsubject config silently upgraded")
	}
	mapping := p.SourceMappings()[0]
	if !p.AllowsObservedDATA(mapping, c.PrincipalAuthority, c.PrincipalNamespace, "local-user-one", c.ServiceID) {
		t.Fatal("explicit local-user policy did not match")
	}
	for _, params := range []struct{ authority, namespace, principal, service string }{{c.SourceOrigin, c.PrincipalNamespace, "local-user-one", c.ServiceID}, {c.IssuerLabel, c.PrincipalNamespace, "local-user-one", c.ServiceID}, {c.PrincipalAuthority, "oidc-subject", "local-user-one", c.ServiceID}, {c.PrincipalAuthority, c.PrincipalNamespace, "email@example.test", c.ServiceID}, {c.PrincipalAuthority, c.PrincipalNamespace, "local-user-one", "other-service"}} {
		if p.AllowsObservedDATA(mapping, params.authority, params.namespace, params.principal, params.service) {
			t.Fatal("scope/identity alias matched")
		}
	}
	c.SourceOrigin = c.PrincipalAuthority
	c.IssuerLabel = c.PrincipalAuthority
	if _, err := ValidateLocalV2(c); err != nil {
		t.Fatal("same URL bytes incorrectly equated semantics or refused explicit namespace", err)
	}
}
func TestLocalV2ImmutableInputOutputAndExactCurrentTuple(t *testing.T) {
	c := localFixture()
	p, err := ValidateLocalV2(c)
	if err != nil {
		t.Fatal(err)
	}
	original := p.SourceMappings()[0]
	c.Bindings[0].PrincipalIDs[0] = "mutated"
	c.Bindings[0].NetworkID = "changed"
	snapshot, ok := p.Snapshot()
	if !ok {
		t.Fatal("missing snapshot")
	}
	snapshot.Bindings[0].PrincipalIDs[0] = "mutated-again"
	snapshot.Bindings[0].NetworkID = "changed-again"
	mappings := p.SourceMappings()
	mappings[0].NetworkID = "changed-third"
	if !p.AllowsObservedDATA(original, "https://local-users.test", contract.PrincipalNamespace, "local-user-one", "service-one") {
		t.Fatal("policy borrowed mutable data")
	}
	altered := original
	altered.MappingRevision = "r2"
	if p.AllowsObservedDATA(altered, "https://local-users.test", contract.PrincipalNamespace, "local-user-one", "service-one") {
		t.Fatal("changed current revision matched")
	}
	if p.AllowsObservedDATA(original, "https://local-users.test", contract.PrincipalNamespace, "mutated", "service-one") {
		t.Fatal("input principal slice was not copied")
	}
	if (*LocalPolicyV2)(nil).AllowsObservedDATA(original, "", "", "", "") || new(LocalPolicyV2).AllowsObservedDATA(original, "", "", "", "") {
		t.Fatal("nil/zero policy matched")
	}
}
func TestLocalV2InvalidAndDuplicateRequiredPolicyData(t *testing.T) {
	for _, kind := range []string{"version", "namespace", "source", "authority", "issuer", "registrar", "revision", "service", "no-bindings", "no-principals", "duplicate-workspace", "duplicate-principal", "principal", "network"} {
		t.Run(kind, func(t *testing.T) {
			c := localFixture()
			switch kind {
			case "version":
				c.Version = 1
			case "namespace":
				c.PrincipalNamespace = "oidc"
			case "source":
				c.SourceOrigin = "http://localhost:1234"
			case "authority":
				c.PrincipalAuthority = "https://local-users.test/path"
			case "issuer":
				c.IssuerLabel = "https://issuer.test?q=x"
			case "registrar":
				c.RegistrarOrigin = "https://registrar.test/"
			case "revision":
				c.Revision = "r 1"
			case "service":
				c.ServiceID = "service.with.dot"
			case "no-bindings":
				c.Bindings = nil
			case "no-principals":
				c.Bindings[0].PrincipalIDs = nil
			case "duplicate-workspace":
				c.Bindings = append(c.Bindings, c.Bindings[0])
			case "duplicate-principal":
				c.Bindings[0].PrincipalIDs = append(c.Bindings[0].PrincipalIDs, c.Bindings[0].PrincipalIDs[0])
			case "principal":
				c.Bindings[0].PrincipalIDs[0] = "user with space"
			case "network":
				c.Bindings[0].NetworkID = "bad/network"
			}
			if _, err := ValidateLocalV2(c); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
	raw, _ := json.Marshal(localFixture())
	var root map[string]any
	json.Unmarshal(raw, &root)
	var walk func(map[string]any)
	walk = func(object map[string]any) {
		for key, value := range object {
			delete(object, key)
			b, _ := json.Marshal(root)
			if _, err := ParseLocalV2(b); err == nil {
				t.Fatal("missing required config field accepted")
			}
			object[key] = nil
			b, _ = json.Marshal(root)
			if _, err := ParseLocalV2(b); err == nil {
				t.Fatal("null config field accepted")
			}
			object[key] = value
			if array, ok := value.([]any); ok {
				for _, entry := range array {
					if nested, ok := entry.(map[string]any); ok {
						walk(nested)
					}
				}
			}
		}
	}
	walk(root)
	for _, bad := range [][]byte{append(raw, []byte(` {}`)...), bytes.Replace(raw, []byte(`"version":2`), []byte(`"version":2,"version":2`), 1), bytes.Replace(raw, []byte(`"principal_authority"`), []byte(`"Principal_Authority"`), 1), bytes.Replace(raw, []byte(`"version":2`), []byte(`"version":2.0`), 1)} {
		if _, err := ParseLocalV2(bad); err == nil {
			t.Fatal("aliased/duplicate/trailing config accepted")
		}
	}
}
