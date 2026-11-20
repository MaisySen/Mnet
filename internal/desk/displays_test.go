package desk

import "testing"

func TestCompiledReviewDisplaysCannotChangeApprovedGeometry(t *testing.T) {
	var missing *Compiled
	if missing.Displays() != nil {
		t.Fatal("nil layout invented geometry")
	}
	spec := runSpecFixture(t)
	compiled, err := Compile(spec.Layout)
	if err != nil {
		t.Fatal(err)
	}
	before := compiled.Fingerprint()
	copy := compiled.Displays()
	copy[0].X = 999
	copy[0].Device = "other"
	copy = append(copy, Display{ID: "extra"})
	if compiled.Fingerprint() != before || len(compiled.Displays()) != 2 || compiled.Displays()[0].X != 0 {
		t.Fatal("review mutated approved arrangement")
	}
}
