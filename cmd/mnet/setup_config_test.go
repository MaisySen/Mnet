package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const setupFixture = `{"version":1,"revision":"fixture-r1","multies_origin":"https://multies.example.test","issuer":"https://identity.example.test/issuer","registrar_origin":"https://registrar.example.test","audience":"fixture-enrollment","bindings":[{"workspace_id":"workspace-fixture","network_id":"net-fixture","principals":[{"issuer":"https://identity.example.test/issuer","subject":"subject-fixture"}]}]}`

func TestOfflineSetupConfigCheck(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "operator.json")
	if e := os.WriteFile(p, []byte(setupFixture), 0600); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e := checkSetupConfig([]string{p}, &out); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"syntax is valid", "Nothing was installed or contacted", "remain unchecked", "not enabled"} {
		if !strings.Contains(out.String(), s) {
			t.Fatal("missing scope warning")
		}
	}
	if strings.Contains(out.String(), "subject-fixture") || strings.Contains(out.String(), "https://") {
		t.Fatal("operator identifiers echoed")
	}
	got, e := os.ReadFile(p)
	if e != nil || string(got) != setupFixture {
		t.Fatal("configuration changed")
	}
	entries, e := os.ReadDir(d)
	if e != nil || len(entries) != 1 {
		t.Fatal("check created state")
	}
	for _, args := range [][]string{nil, {}, {p, p}, {""}, {d}, {filepath.Join(d, "absent")}} {
		out.Reset()
		if e := checkSetupConfig(args, &out); e == nil || out.Len() != 0 {
			t.Fatal("invalid invocation accepted")
		}
	}
	if e := os.WriteFile(p, []byte(`{"version":1,"version":1}`), 0600); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	if e := checkSetupConfig([]string{p}, &out); e == nil || out.Len() != 0 {
		t.Fatal("ambiguous config accepted")
	}
}
