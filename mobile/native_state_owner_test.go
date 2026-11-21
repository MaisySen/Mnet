package mobile

import "testing"

func TestMobileNodeOwnsStateAcrossStopUntilClose(t *testing.T) {
	dir := t.TempDir()
	first, err := NewNode(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := NewNode(dir); err == nil {
		second.Close()
		t.Fatal("mobile admitted second identity owner")
	}
	first.Stop()
	if second, err := NewNode(dir); err == nil {
		second.Close()
		t.Fatal("mobile Stop released reusable identity owner")
	}
	first.Close()
	second, err := NewNode(dir)
	if err != nil {
		t.Fatal("mobile Close did not release owner", err)
	}
	second.Close()
}
