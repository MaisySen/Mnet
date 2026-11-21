package enrollmentprovisioning

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func sample(t *testing.T) []byte {
	t.Helper()
	raw, e := os.ReadFile("testdata/plan.json")
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestPlanConsistencyNeverAuthority(t *testing.T) {
	raw := sample(t)
	before := append([]byte(nil), raw...)
	r, e := Verify(raw)
	if e != nil {
		t.Fatal(e)
	}
	if !r.ConfigurationConsistent || r.AuthorityVerified || r.Deployable || r.WorkspaceCount != 2 || len(r.PlanSHA256) != 64 {
		t.Fatal("misleading result", r)
	}
	for _, c := range r.RequiredChecks {
		if c.Owner == "" || c.Requirement == "" || (c.Status != "unverified" && c.Status != "unresolved") {
			t.Fatal("claimed external authority", c)
		}
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("mutated caller input")
	}
	var p Plan
	json.Unmarshal(raw, &p)
	// Reordering sets changes no meaning/hash. JSON whitespace/order immaterial.
	p.Native.Policy.Bindings[0], p.Native.Policy.Bindings[1] = p.Native.Policy.Bindings[1], p.Native.Policy.Bindings[0]
	p.Multies.Bindings[0], p.Multies.Bindings[1] = p.Multies.Bindings[1], p.Multies.Bindings[0]
	p.Registrar.Allowed[0], p.Registrar.Allowed[1] = p.Registrar.Allowed[1], p.Registrar.Allowed[0]
	p.WorkspaceOwners[0], p.WorkspaceOwners[1] = p.WorkspaceOwners[1], p.WorkspaceOwners[0]
	shuffled, _ := json.Marshal(p)
	other, e := Verify(shuffled)
	if e != nil || !reflect.DeepEqual(r, other) {
		t.Fatal("nondeterministic report", e)
	}
	p.OperatorOwnerRef = "different-original-source"
	changed, _ := json.Marshal(p)
	other, e = Verify(changed)
	if e != nil || other.PlanSHA256 == r.PlanSHA256 {
		t.Fatal("hash ignored approval-source reference")
	}
}
func TestPlanBoundariesAndMismatchRefusal(t *testing.T) {
	tests := map[string]func(*Plan){
		"legacy-open":   func(p *Plan) { p.LegacyBoundary = "legacy-open-same-network" },
		"rotation":      func(p *Plan) { p.Change = "rotate" },
		"authority":     func(p *Plan) { p.Multies.Provenance.Authority = "https://other.example.test" },
		"namespace":     func(p *Plan) { p.Multies.Provenance.Namespace = "oidc-subject" },
		"revision":      func(p *Plan) { p.Multies.Revision = "new-r2" },
		"audience":      func(p *Plan) { p.Service.Audience = "other" },
		"origin":        func(p *Plan) { p.Registrar.SourceOrigin = "https://other.example.test" },
		"service":       func(p *Plan) { p.Service.ServiceID = "other" },
		"key":           func(p *Plan) { p.Service.KeyID = "rotated-key" },
		"mapping":       func(p *Plan) { p.Registrar.Allowed[0].NetworkID = "network-b" },
		"principals":    func(p *Plan) { p.Multies.Bindings[0].Principals = []string{"not-a-member"} },
		"duplicate":     func(p *Plan) { p.WorkspaceOwners[1] = p.WorkspaceOwners[0] },
		"owner":         func(p *Plan) { p.WorkspaceOwners[0].OwnerRef = "" },
		"selfbootstrap": func(p *Plan) { p.Native.TrustOwnerRef = "" },
		"zeroroot":      func(p *Plan) { p.Native.TrustRootSHA256 = strings.Repeat("0", 64) },
		"sharednetwork": func(p *Plan) {
			p.Native.Policy.Bindings[1].NetworkID = "network-a"
			p.Multies.Bindings[1].Network = "network-a"
			p.Registrar.Allowed[1].NetworkID = "network-a"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var p Plan
			json.Unmarshal(sample(t), &p)
			mutate(&p)
			raw, _ := json.Marshal(p)
			r, e := Verify(raw)
			if e == nil || r.ConfigurationConsistent {
				t.Fatal("bad plan accepted")
			}
		})
	}
}
func TestPlanExactBoundedSecretFreeSchema(t *testing.T) {
	raw := sample(t)
	inputs := [][]byte{nil, bytes.Repeat([]byte{'x'}, MaxBytes+1), []byte(`null`), append(append([]byte(nil), raw...), []byte(`{}`)...), bytes.Replace(raw, []byte(`"version": 1`), []byte(`"version": 1.0`), 1), bytes.Replace(raw, []byte(`"version": 1`), []byte(`"version":1,"version":1`), 1), bytes.Replace(raw, []byte(`"version": 1`), []byte(`"Version":1`), 1), bytes.Replace(raw, []byte(`"version": 1`), []byte(`"version":1,"secret":"never-accept"`), 1), bytes.Replace(raw, []byte(`"version": 1`), []byte(`"version":1,"signature":"unsigned-plan-is-not-verification"`), 1)}
	for i, input := range inputs {
		if _, e := Verify(input); e == nil {
			t.Fatal("malformed/secret input accepted", i)
		}
	}
}

func TestPlanEveryRequiredFieldAndUnknownNestedRefuses(t *testing.T) {
	var original map[string]any
	if e := json.Unmarshal(sample(t), &original); e != nil {
		t.Fatal(e)
	}
	var walk func(any, []any)
	replace := func(path []any, remove bool) {
		var clone any
		json.Unmarshal(sample(t), &clone)
		current := clone
		for _, part := range path[:len(path)-1] {
			switch k := part.(type) {
			case string:
				current = current.(map[string]any)[k]
			case int:
				current = current.([]any)[k]
			}
		}
		last := path[len(path)-1].(string)
		if remove {
			delete(current.(map[string]any), last)
		} else {
			current.(map[string]any)[last] = nil
		}
		raw, _ := json.Marshal(clone)
		if _, e := Verify(raw); e == nil {
			t.Fatalf("missing/null field accepted %v", path)
		}
	}
	walk = func(value any, path []any) {
		switch v := value.(type) {
		case map[string]any:
			for k, item := range v {
				next := append(append([]any(nil), path...), k)
				replace(next, true)
				replace(next, false)
				walk(item, next)
			}
		case []any:
			for i, item := range v {
				walk(item, append(append([]any(nil), path...), i))
			}
		}
	}
	walk(original, nil)
}
