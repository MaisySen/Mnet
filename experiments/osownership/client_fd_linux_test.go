//go:build linux && mnet_enrollment_development

package osownership

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRawFDInventoryOnlyExactOwnedDirectoryExcluded(t *testing.T) {
	for _, c := range []struct {
		name     string
		names    []string
		owned    int
		want     []int
		parseErr bool
		allow    bool
	}{{"strict", []string{"0", "1", "2", "3"}, 3, []int{0, 1, 2}, false, true}, {"foreign-fd3", []string{"0", "1", "2", "3", "4"}, 4, []int{0, 1, 2, 3}, false, false}, {"runtime-poll-not-owned", []string{"0", "1", "2", "3", "5", "6"}, 3, []int{0, 1, 2, 5, 6}, false, false}, {"missing-custody", []string{"0", "1", "2"}, 3, nil, true, false}, {"duplicate", []string{"0", "1", "2", "3", "3"}, 3, nil, true, false}, {"invalid", []string{"0", "1", "2", "bad", "3"}, 3, nil, true, false}} {
		t.Run(c.name, func(t *testing.T) {
			got, e := parseOwnedFDInventory(c.names, c.owned)
			if (e != nil) != c.parseErr {
				t.Fatal(e)
			}
			if e != nil {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatal(got)
			}
			e = validateFDAllowlist(got)
			if (e == nil) != c.allow {
				t.Fatal("strict allowlist changed", e)
			}
			if !c.allow {
				var harness *FDInventoryHarnessError
				if !errors.As(e, &harness) {
					t.Fatal("fd error not harness classified")
				}
			}
		})
	}
}
func TestClientEvidenceFDOrderingRawNoPollRegistration(t *testing.T) {
	s := sourceFunction(t, "client_sandbox_linux.go", "clientFDInventorySource")
	for _, bad := range []string{"os.Open(", "os.ReadDir(", "os.ReadFile(", "os.NewFile("} {
		if strings.Contains(s, bad) {
			t.Fatal("poll-registering inventory operation", bad)
		}
	}
	for _, need := range []string{"unix.Open(", "unix.Getdents(", "parseOwnedFDInventory(names, fd)"} {
		if !strings.Contains(s, need) {
			t.Fatal("missing raw custody path", need)
		}
	}
	s = sourceFunction(t, "bootstrap_evidence_linux.go", "emitClientBootstrapEvidence")
	a := strings.Index(s, "clientFDInventorySource()")
	b := strings.Index(s, "rawReadBounded(")
	if a < 0 || b < a {
		t.Fatal("fd inventory not first")
	}
	if strings.Contains(s, "os.ReadFile(") || strings.Contains(s, "numericCurrentNamespace()") {
		t.Fatal("client initializes runtime poll before fd evidence")
	}
}
