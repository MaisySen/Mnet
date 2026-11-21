package main

import (
	"strings"
	"testing"
)

const deskFixture = `{"version":1,"displays":[{"id":"a","device":"left","x":-100,"y":0,"width":100,"height":100,"pixel_width":1000,"pixel_height":1000},{"id":"b","device":"right","x":0,"y":0,"width":100,"height":100,"pixel_width":1500,"pixel_height":1500}]}`

func TestDeskLayoutCheckIsOfflineCandidateOnly(t *testing.T) {
	report, err := checkDeskLayout(strings.NewReader(deskFixture), []string{"--edge", "a", "right", "500"})
	if err != nil || !report.OfflineOnly || report.Active || report.Candidate == nil || report.Candidate.Y != 750 || report.Candidate.X != 0 {
		t.Fatalf("invalid offline report %+v %v", report, err)
	}
}
func TestDeskLayoutCheckRefusesMalformedInput(t *testing.T) {
	for _, data := range []string{deskFixture + "{}", strings.Replace(deskFixture, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(deskFixture, `"version":1`, `"grant":true,"version":1`, 1), strings.Repeat("[", 10) + strings.Repeat("]", 10), strings.Repeat(" ", 65537)} {
		if _, err := checkDeskLayout(strings.NewReader(data), nil); err == nil {
			t.Fatal("invalid offline file accepted")
		}
	}
	for _, query := range [][]string{{"--pair", "a", "right", "500"}, {"--edge", "a", "right"}, {"--edge", "a", "right", "1000"}, {"--edge", "a", "right", "NaN"}} {
		if _, err := checkDeskLayout(strings.NewReader(deskFixture), query); err == nil {
			t.Fatal("invalid query accepted", query)
		}
	}
}

func TestValidDeskArgsLayoutNew(t *testing.T) {
	valid := [][]string{
		{"layout-new", "--out", "new.json"},
		{"layout-edit", "layout.json", "--out", "new.json"},
	}
	for _, args := range valid {
		if !validDeskArgs(args) {
			t.Fatalf("valid args rejected: %v", args)
		}
	}
	invalid := [][]string{
		{"layout-new"},
		{"layout-new", "new.json"},
		{"layout-new", "--out"},
		{"layout-new", "--out", "new.json", "extra"},
		{"layout-new", "layout.json", "--out", "new.json"},
	}
	for _, args := range invalid {
		if validDeskArgs(args) {
			t.Fatalf("invalid args accepted: %v", args)
		}
	}
}
