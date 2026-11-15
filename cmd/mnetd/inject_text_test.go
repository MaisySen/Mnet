//go:build linux

package main

import "testing"

func TestResolveTextMode(t *testing.T) {
	haveAll := func(string) bool { return true }
	haveNone := func(string) bool { return false }
	cases := []struct {
		req, backend string
		have         func(string) bool
		want         string
	}{
		{"", "x11", haveNone, "keysym"},
		{"auto", "wayland", haveNone, "ydotool"},
		{"clipboard", "x11", haveAll, "clipboard"},
		{"clipboard", "x11", haveNone, "keysym"},
		{"clipboard", "wayland", haveNone, "ydotool"},
		{"keysym", "x11", haveNone, "keysym"},
		{"bogus", "x11", haveNone, "keysym"},
		{"bogus", "wayland", haveNone, "ydotool"},
	}
	for _, c := range cases {
		if got := resolveTextMode(c.req, c.backend, c.have); got != c.want {
			t.Errorf("resolveTextMode(%q,%q) = %q, want %q", c.req, c.backend, got, c.want)
		}
	}
}

func TestKeysymForRune(t *testing.T) {
	if k := keysymForRune('a'); k != 'a' {
		t.Errorf("ascii a: %#x", uint32(k))
	}
	if k := keysymForRune(' '); k != 0x20 {
		t.Errorf("space: %#x", uint32(k))
	}
	// Non-ASCII uses the Unicode keysym range.
	if k := keysymForRune('é'); k != 0x010000e9 {
		t.Errorf("e-acute: %#x", uint32(k))
	}
	if k := keysymForRune('雪'); k != 0x010096ea {
		t.Errorf("CJK: %#x", uint32(k))
	}
}
