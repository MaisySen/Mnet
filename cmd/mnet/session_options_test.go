package main

import "testing"

func TestSessionConnectOptionsRejectMalformed(t *testing.T) {
	for _, args := range [][]string{
		{"--unknown", "x"}, {"--save-frames"}, {"--send-file"},
		{"--save-frames", "--send-file", "secret.bin"},
		{"--send-file", ""}, {"--save-frames", ""},
		{"--save-frames", "a", "--save-frames", "b"},
		{"--send-file", "a", "--send-file", "b"},
		{"extra"}, {"--save-frames", "dir", "trailing"},
	} {
		if _, _, err := parseSessionConnectOptions(args); err == nil {
			t.Errorf("accepted malformed options: %q", args)
		}
	}
}
func TestSessionConnectOptionsPreserveExactValues(t *testing.T) {
	for _, args := range [][]string{
		{"--save-frames", "frames with spaces", "--send-file", "./--private.bin"},
		{"--send-file", "./--private.bin", "--save-frames", "frames with spaces"},
	} {
		dir, file, err := parseSessionConnectOptions(args)
		if err != nil || dir != "frames with spaces" || file != "./--private.bin" {
			t.Fatalf("values changed: %q %q %v", dir, file, err)
		}
	}
	dir, file, err := parseSessionConnectOptions(nil)
	if err != nil || dir != "" || file != "" {
		t.Fatalf("empty options: %q %q %v", dir, file, err)
	}
}
