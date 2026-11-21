//go:build linux && mnet_enrollment_development

package osownership

import (
	"strings"
	"testing"
)

func TestStartTicksParserPIDAndComplexComm(t *testing.T) {
	stat := "123 (name ) with spaces) S " + strings.Repeat("0 ", 18) + "987 0"
	ticks, e := parseProcessStartTicks(stat, 123)
	if e != nil || ticks != 987 {
		t.Fatal(ticks, e)
	}
	for _, s := range []string{"", strings.Replace(stat, "123 ", "124 ", 1), "123 (bad) S", strings.Replace(stat, "987", "0", 1)} {
		if _, e := parseProcessStartTicks(s, 123); e == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}
