//go:build linux && mnet_enrollment_development

package osownership

import (
	"strings"
	"testing"
)

func TestClientStatusZeroCapsAndNoNewPrivsEachField(t *testing.T) {
	valid := "CapInh: 0000000000000000\nCapPrm: 0000000000000000\nCapEff: 0000000000000000\nCapBnd: 0000000000000000\nCapAmb: 0000000000000000\nNoNewPrivs: 1\nSeccomp: 2\n"
	if e := validateClientStatus(valid); e != nil {
		t.Fatal(e)
	}
	for _, k := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
		bad := strings.Replace(valid, k+": 0000000000000000", k+": 0000000000000001", 1)
		if validateClientStatus(bad) == nil {
			t.Fatal("nonzero capability accepted", k)
		}
		bad = strings.Replace(valid, k+": 0000000000000000\n", "", 1)
		if validateClientStatus(bad) == nil {
			t.Fatal("missing cap evidence accepted")
		}
	}
	if validateClientStatus(strings.Replace(valid, "NoNewPrivs: 1", "NoNewPrivs: 0", 1)) == nil {
		t.Fatal("nnp off accepted")
	}
}
