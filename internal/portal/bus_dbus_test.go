//go:build linux

package portal

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestNormalize(t *testing.T) {
	in := map[string]dbus.Variant{
		"session_handle": dbus.MakeVariant(dbus.ObjectPath("/s/1")),
		"fd":             dbus.MakeVariant(dbus.UnixFD(9)),
		"n":              dbus.MakeVariant(uint32(3)),
	}
	got := normalize(in).(map[string]any)
	if got["session_handle"] != "/s/1" {
		t.Fatalf("session_handle = %v (%T)", got["session_handle"], got["session_handle"])
	}
	if got["fd"] != 9 {
		t.Fatalf("fd = %v (%T)", got["fd"], got["fd"])
	}
	if got["n"] != uint32(3) {
		t.Fatalf("n = %v", got["n"])
	}
}

func TestNormalizeStructSlice(t *testing.T) {
	// godbus decodes a(ua{sv}) into a slice of anonymous structs;
	// a named struct slice exercises the same reflection path.
	type entry struct {
		Node  uint32
		Props map[string]dbus.Variant
	}
	in := []entry{
		{Node: 42, Props: map[string]dbus.Variant{"size": dbus.MakeVariant([]int32{1920, 1080})}},
	}
	got, ok := normalize(in).([]any)
	if !ok {
		t.Fatalf("normalize gave %T", normalize(in))
	}
	row, ok := got[0].([]any)
	if !ok || len(row) != 2 {
		t.Fatalf("row shape %T len", got[0])
	}
	if row[0] != uint32(42) {
		t.Fatalf("node = %v", row[0])
	}
	props, ok := row[1].(map[string]any)
	if !ok {
		t.Fatalf("props = %T", row[1])
	}
	size, ok := props["size"].([]any)
	if !ok || !reflect.DeepEqual(size, []any{int32(1920), int32(1080)}) {
		t.Fatalf("size = %v", props["size"])
	}
}

func TestNormalizePassThrough(t *testing.T) {
	if normalize(7) != 7 {
		t.Fatal("scalar changed")
	}
	s := []any{"a", 1}
	out := normalize(s).([]any)
	if out[0] != "a" || out[1] != 1 {
		t.Fatalf("[]any changed: %v", out)
	}
}
