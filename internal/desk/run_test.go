package desk

import (
	"encoding/json"
	"strings"
	"testing"
)

func runSpecFixture(t *testing.T) RunSpec {
	t.Helper()
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	l := Layout{Version: 1, Displays: []Display{{ID: "source", Device: a, Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768}, {ID: "target", Device: b, X: 100, Width: 100, Height: 100, PixelWidth: 1024, PixelHeight: 768}}}
	c, err := Compile(l)
	if err != nil {
		t.Fatal(err)
	}
	return RunSpec{Version: 1, Role: "primary", Address: "127.0.0.1:4567", Certificate: "cert.pem", PrivateKey: "key.pem", RunDirectory: "runs", LifetimeSeconds: 30, SourceDisplay: "source", Edge: Right, Binding: Binding{Run: strings.Repeat("c", 64), Primary: a, Recipient: b, Layout: c.Fingerprint(), Keyboard: strings.Repeat("d", 64), Display: "target"}, Layout: l}
}
func TestDecodeRunExactAndFailClosed(t *testing.T) {
	spec := runSpecFixture(t)
	data, _ := json.Marshal(spec)
	if got, _, err := DecodeRun(strings.NewReader(string(data))); err != nil || got.Binding != spec.Binding {
		t.Fatal(got, err)
	}
	for _, bad := range []string{string(data) + "{}", strings.Replace(string(data), `"version":1`, `"version":1,"version":1`, 1), strings.Replace(string(data), `"version":1`, `"grant":true,"version":1`, 1), strings.Replace(string(data), `"role"`, `"Role"`, 1), strings.Repeat(" ", 65537)} {
		if _, _, err := DecodeRun(strings.NewReader(bad)); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
	cases := []func(*RunSpec){func(s *RunSpec) { s.Role = "auto" }, func(s *RunSpec) { s.LifetimeSeconds = 0 }, func(s *RunSpec) { s.LifetimeSeconds = 601 }, func(s *RunSpec) { s.Address = "0.0.0.0:4567" }, func(s *RunSpec) { s.Address = "localhost:4567" }, func(s *RunSpec) { s.Address = "224.0.0.1:4567" }, func(s *RunSpec) { s.Address = "255.255.255.255:4567" }, func(s *RunSpec) { s.Address = "127.0.0.1:0" }, func(s *RunSpec) { s.SourceDisplay = "target" }, func(s *RunSpec) { s.Edge = Left }, func(s *RunSpec) { s.Binding.Primary = "label" }, func(s *RunSpec) { s.RunDirectory = "" }}
	for _, change := range cases {
		s := spec
		change(&s)
		if _, err := s.Validate(); err == nil {
			t.Fatal("invalid explicit run accepted", s)
		}
	}
}
