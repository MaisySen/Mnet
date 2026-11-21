//go:build mnet_enrollment_development || multies_native_fixture

package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fintex.au/mnet/internal/netmap"
	"os"
	"testing"
)

func developmentConfigurationFixture(t *testing.T, d *DisposableRegistrar) ([]byte, []byte, string) {
	t.Helper()
	f := newWorkflowFixture(t, "https://127.0.0.1:4567")
	defer f.s.Close()
	raw, e := os.ReadFile(f.s.path + ".registrar-sources-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var sources registrarSourceConfig
	json.Unmarshal(raw, &sources)
	origin := "https://127.0.0.1:4568"
	sources.Sources[0].RegistrarOrigin = origin
	sources.Sources[0].Allowed[0].RegistrarOrigin = origin
	sources.Sources[0].Allowed[0].NetworkID = d.network
	workflow := f.policy
	workflow.Policy.RegistrarOrigin = origin
	workflow.Policy.Bindings[0].NetworkID = d.network
	a, _ := json.Marshal(sources)
	b, _ := json.Marshal(workflow)
	return a, b, origin
}
func TestDevelopmentConfigurationOriginalProfileAndInitialOnly(t *testing.T) {
	d, e := NewDisposableRegistrar(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	sources, workflow, origin := developmentConfigurationFixture(t, d)
	if _, e = d.validateDevelopmentConfiguration(sources, workflow, origin); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"clock_uncertainty_ms", "challenge_ms", "max_lease_ms", "max_observation_age_ms", "max_live_per_principal", "max_proof_attempts", "max_live", "max_attempts", "max_records", "max_state_bytes"} {
		var fields map[string]json.RawMessage
		json.Unmarshal(workflow, &fields)
		fields[field] = []byte("2000000")
		bad, _ := json.Marshal(fields)
		if _, e = d.validateDevelopmentConfiguration(sources, bad, origin); e == nil {
			t.Fatalf("widened %s", field)
		}
	}
	if _, e = d.validateDevelopmentConfiguration(sources, workflow, "https://public.example.test"); e == nil {
		t.Fatal("public registrar accepted")
	}
	if e = d.Configure(sources, workflow, origin); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(d.store.path + ".registrar-workflow-v1.json")
	if e = d.Configure(sources, workflow, origin); e == nil {
		t.Fatal("configuration reinstall accepted")
	}
	after, _ := os.ReadFile(d.store.path + ".registrar-workflow-v1.json")
	if !bytes.Equal(before, after) {
		t.Fatal("reinstall rewrote")
	}
	if _, e = d.Handler(); e != nil {
		t.Fatal(e)
	}
	if _, e = d.Handler(); e == nil {
		t.Fatal("second gate allowed")
	}
}
func TestDevelopmentConfigurationEnvelopeExact(t *testing.T) {
	valid := []byte(`{"sources":{},"workflow":{}}`)
	if _, e := DecodeDisposableRegistrarConfiguration(valid); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{`{}`, `{"sources":{},"sources":{},"workflow":{}}`, `{"Sources":{},"workflow":{}}`, `{"sources":{},"workflow":{},"extra":0}`, `{"sources":{},"workflow":{}}{}`, `null`, `{"sources":{}}`} {
		if _, e := DecodeDisposableRegistrarConfiguration([]byte(bad)); e == nil {
			t.Fatal("nonexact envelope accepted")
		}
	}
}
func TestDevelopmentRootRefusesPrefixedLegacyIdentities(t *testing.T) {
	for _, kind := range []string{"legacy-node", "auth-key", "token", "identity", "membership"} {
		t.Run(kind, func(t *testing.T) {
			d, e := NewDisposableRegistrar(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			root := d.root
			switch kind {
			case "auth-key":
				_, e = d.store.CreateAuthKey(d.network, "legacy", false, 1)
			case "legacy-node":
				_, e = d.store.Join(netmap.JoinRequest{Network: d.network, Name: "legacy", PubKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))})
			case "token":
				d.store.data.NodeTokens["legacy"] = "token"
				e = d.store.saveLocked()
			case "identity":
				d.store.data.ApprovalIdentities = map[string]ApprovalIdentity{"legacy": {Issuer: "https://issuer.test", Subject: "legacy"}}
				e = d.store.saveLocked()
			case "membership":
				d.store.data.Memberships = map[string]NetworkMember{"legacy": {}}
				e = d.store.saveLocked()
			}
			if e != nil {
				t.Fatal(e)
			}
			before, _ := os.ReadFile(d.store.path)
			path := d.store.path
			d.Close()
			if adopted, e := OpenDisposableRegistrar(root); e == nil {
				adopted.Close()
				t.Fatal("legacy identity adopted")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("refusal rewrote")
			}
		})
	}
}
