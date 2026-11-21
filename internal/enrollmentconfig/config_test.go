package enrollmentconfig

import (
	"errors"
	"reflect"
	"testing"
)

func fixture() Config {
	return Config{Version: 1, Revision: "fixture-r1", MultiesOrigin: "https://multies.example.test", Issuer: "https://identity.example.test/issuer", RegistrarOrigin: "https://registrar.example.test:8443", Audience: "fixture-enrollment", Bindings: []Binding{{WorkspaceID: "workspace-fixture", NetworkID: "net-fixture", Principals: []Principal{{Issuer: "https://identity.example.test/issuer", Subject: "subject-fixture"}}}}}
}
func TestExactMappingAndPrivateCopies(t *testing.T) {
	c := fixture()
	m, e := Validate(c)
	if e != nil {
		t.Fatal(e)
	}
	want := c.Bindings[0]
	want.Principals = append([]Principal(nil), want.Principals...)
	c.Bindings[0].Principals[0].Subject = "changed"
	c.Bindings[0].NetworkID = "changed"
	got, ok := m.Lookup("workspace-fixture")
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatal("input mutation changed mapping")
	}
	got.Principals[0].Subject = "changed-again"
	got, _ = m.Lookup("workspace-fixture")
	if !reflect.DeepEqual(got, want) {
		t.Fatal("output mutation changed mapping")
	}
	for _, s := range []string{"", "workspace-fixture ", "WORKSPACE-FIXTURE", "net-fixture", "subject-fixture"} {
		if _, ok = m.Lookup(s); ok {
			t.Fatal("inexact workspace resolved")
		}
	}
	if _, ok := (*Mapping)(nil).Lookup("workspace-fixture"); ok {
		t.Fatal("nil mapping resolved")
	}
	if _, ok := new(Mapping).Lookup("workspace-fixture"); ok {
		t.Fatal("zero mapping resolved")
	}
}
func TestRejectInvalidOperatorHandoffs(t *testing.T) {
	cases := map[string]func(*Config){
		"zero":                func(c *Config) { *c = Config{} },
		"version":             func(c *Config) { c.Version = 2 },
		"revision":            func(c *Config) { c.Revision = "" },
		"no binding":          func(c *Config) { c.Bindings = nil },
		"workspace":           func(c *Config) { c.Bindings[0].WorkspaceID = "" },
		"network":             func(c *Config) { c.Bindings[0].NetworkID = "" },
		"no principals":       func(c *Config) { c.Bindings[0].Principals = nil },
		"subject":             func(c *Config) { c.Bindings[0].Principals[0].Subject = "" },
		"issuer mismatch":     func(c *Config) { c.Bindings[0].Principals[0].Issuer = "https://other.example.test" },
		"duplicate principal": func(c *Config) { b := &c.Bindings[0]; b.Principals = append(b.Principals, b.Principals[0]) },
		"duplicate workspace": func(c *Config) {
			b := c.Bindings[0]
			b.NetworkID = "different-network"
			c.Bindings = append(c.Bindings, b)
		},
		"audience": func(c *Config) { c.Audience = "" },
		"control":  func(c *Config) { c.Bindings[0].NetworkID = "bad\x00id" },
		"space":    func(c *Config) { c.Bindings[0].NetworkID = "bad id" },
		"too many": func(c *Config) { c.Bindings = make([]Binding, 1025) },
	}
	for _, s := range []string{"", "http://multies.example.test", "https://user:password@host.test", "https://host.test/", "https://host.test/path", "https://host.test?x=1", "https://host.test?", "https://host.test#", "https://host.test#fragment", "https://host.test:", "https://host.test:0", "https://host.test:99999", " https://host.test", "https://host.test\n", "//host.test"} {
		s := s
		cases["origin "+s] = func(c *Config) { c.MultiesOrigin = s }
		cases["registrar "+s] = func(c *Config) { c.RegistrarOrigin = s }
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := fixture()
			mutate(&c)
			m, e := Validate(c)
			if m != nil || !errors.Is(e, ErrInvalid) {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestDistinctWorkspaceMappingsRemainExplicit(t *testing.T) {
	c := fixture()
	b := c.Bindings[0]
	b.WorkspaceID = "second-workspace"
	c.Bindings = append(c.Bindings, b)
	m, e := Validate(c)
	if e != nil {
		t.Fatal(e)
	}
	if b, ok := m.Lookup("second-workspace"); !ok || b.NetworkID != "net-fixture" {
		t.Fatal("explicit shared-network mapping lost")
	}
}
