package enrollmentconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseExactOperatorSchema(t *testing.T) {
	raw, e := json.Marshal(fixture())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Parse(raw); e != nil {
		t.Fatal(e)
	}
	base := string(raw)
	cases := []string{
		"null", "[]", "{}", base + "{}", base + " garbage",
		strings.Replace(base, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(base, `"version":1`, `"Version":1`, 1),
		strings.Replace(base, `"version":1`, `"version":null`, 1),
		strings.Replace(base, `"version":1`, `"version":"1"`, 1),
		strings.Replace(base, `"version":1`, `"version":1,"unexpected":true`, 1),
		strings.Replace(base, `"revision":"fixture-r1",`, "", 1),
		strings.Replace(base, `"workspace_id":"workspace-fixture"`, `"workspace_id":"workspace-fixture","workspace_\u0069d":"changed"`, 1),
		strings.Replace(base, `"subject":"subject-fixture"`, `"subject":"subject-fixture","subject":"another"`, 1),
		strings.Replace(base, `"subject":"subject-fixture"`, `"Subject":"subject-fixture"`, 1),
		strings.Replace(base, `"subject":"subject-fixture"`, `"subject":[]`, 1),
		strings.Replace(base, `"bindings":[`, `"bindings":null,"ignored":[`, 1),
		string([]byte{0xff}), strings.Repeat(" ", MaxBytes+1),
	}
	for i, s := range cases {
		if m, e := Parse([]byte(s)); e == nil || m != nil {
			t.Fatalf("invalid document accepted: case %d", i)
		}
	}
	// JSON escape aliases with no duplicate are ordinary equivalent field names.
	escaped := strings.Replace(base, `"workspace_id"`, `"workspace_\u0069d"`, 1)
	if _, e := Parse([]byte(escaped)); e != nil {
		t.Fatal("decoded exact name rejected")
	}
}
func FuzzParseOperatorConfig(f *testing.F) {
	raw, _ := json.Marshal(fixture())
	f.Add(raw)
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, b []byte) {
		m, e := Parse(b)
		if e != nil && m != nil {
			t.Fatal("mapping returned with failure")
		}
		if e == nil && m == nil {
			t.Fatal("nil mapping accepted")
		}
	})
}
