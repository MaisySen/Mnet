package desk

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func preparationFixture() Preparation {
	return Preparation{Version: 1, Primary: PreparationEndpoint{Inspection: Inspection{true, false, strings.Repeat("a", 64), strings.Repeat("c", 64), 1024, 768}, Certificate: filepath.Join(os.TempDir(), "primary", "cert.pem"), PrivateKey: filepath.Join(os.TempDir(), "primary", "key.pem")}, Recipient: PreparationEndpoint{Inspection: Inspection{true, false, strings.Repeat("b", 64), strings.Repeat("c", 64), 1280, 960}, Certificate: filepath.Join(os.TempDir(), "recipient", "cert.pem"), PrivateKey: filepath.Join(os.TempDir(), "recipient", "key.pem")}, Address: "192.0.2.4:5910", Edge: Right, LifetimeSeconds: 120}
}
func TestPreparationTwoExactDraftsEveryDirection(t *testing.T) {
	for _, edge := range []Edge{Left, Right, Top, Bottom} {
		request := preparationFixture()
		request.Edge = edge
		before := request
		result, err := Prepare(request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.DraftOnly || !reflect.DeepEqual(before, request) {
			t.Fatal("preparation mutated input or implied activation")
		}
		if result.Primary.Role != "primary" || result.Recipient.Role != "recipient" || result.Primary.Binding != result.Recipient.Binding || !reflect.DeepEqual(result.Primary.Layout, result.Recipient.Layout) {
			t.Fatal("draft binding disagreement")
		}
		if result.Primary.Certificate != request.Primary.Certificate || result.Recipient.Certificate != request.Recipient.Certificate || result.Primary.RunDirectory != "replay" || result.Recipient.RunDirectory != "replay" {
			t.Fatal("local paths mixed")
		}
		for _, spec := range []RunSpec{result.Primary, result.Recipient} {
			if _, err = spec.Validate(); err != nil {
				t.Fatal(err)
			}
		}
		next, err := Prepare(request)
		if err != nil || next.Primary.Binding.Run == result.Primary.Binding.Run {
			t.Fatal("run reused", err)
		}
	}
}
func TestPreparationRefusalsReturnNoPartialDraft(t *testing.T) {
	cases := map[string]func(*Preparation){
		"version": func(p *Preparation) { p.Version = 2 }, "claimed-approval": func(p *Preparation) { p.Primary.Inspection.Approval = true }, "not-local": func(p *Preparation) { p.Recipient.Inspection.LocalEvidenceOnly = false }, "same-key": func(p *Preparation) { p.Recipient.Inspection.CertificateKey = p.Primary.Inspection.CertificateKey }, "map": func(p *Preparation) { p.Recipient.Inspection.Keyboard = strings.Repeat("d", 64) }, "invalid-pin": func(p *Preparation) { p.Primary.Inspection.CertificateKey = strings.Repeat("A", 64) }, "small": func(p *Preparation) { p.Primary.Inspection.Width = 63 }, "huge": func(p *Preparation) { p.Recipient.Inspection.Height = 32768 }, "relative-key": func(p *Preparation) { p.Primary.PrivateKey = "key.pem" }, "same-file": func(p *Preparation) { p.Recipient.PrivateKey = p.Recipient.Certificate }, "wildcard": func(p *Preparation) { p.Address = "0.0.0.0:5910" }, "dns": func(p *Preparation) { p.Address = "device.invalid:5910" }, "edge": func(p *Preparation) { p.Edge = "diagonal" }, "lifetime": func(p *Preparation) { p.LifetimeSeconds = 601 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			request := preparationFixture()
			change(&request)
			got, err := prepare(request, nil)
			if err == nil || !reflect.DeepEqual(got, PreparedRun{}) {
				t.Fatal("invalid request produced draft", got, err)
			}
		})
	}
	if got, err := prepare(preparationFixture(), bytes.NewReader(make([]byte, 31))); err == nil || !reflect.DeepEqual(got, PreparedRun{}) {
		t.Fatal("short randomness returned partial run")
	}
}
func TestPreparationStrictDecode(t *testing.T) {
	data, _ := json.Marshal(preparationFixture())
	if _, err := DecodePreparation(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{[]byte(`{"version":1,"version":1}`), []byte(`{"unknown":true}`), append(data, []byte(`{}`)...), bytes.Repeat([]byte(" "), 65537)} {
		if _, err := DecodePreparation(bytes.NewReader(raw)); err == nil {
			t.Fatal("invalid schema accepted")
		}
	}
}
