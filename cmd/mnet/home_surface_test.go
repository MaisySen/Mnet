package main

import (
	"strings"
	"testing"
)

func TestHomeSurfaceAllRoutesAndNoticesFit(t *testing.T) {
	for _, notice := range []string{"Choose a tool to begin.", "No screen arrived. The session ended. Choose Remote Desktop to try again.", "The tool could not finish. Choose it again to try a fresh attempt.", "Tool closed. You can choose another tool."} {
		for _, layout := range []bool{false, true} {
			for focus := -1; focus < 4; focus++ {
				im, err := renderHomeSurface(notice, layout, focus)
				if err != nil {
					t.Fatal(err)
				}
				if im.Bounds().Dx() != 900 || im.Bounds().Dy() != 560 {
					t.Fatal("home dimensions changed")
				}
			}
		}
	}
	if _, err := renderHomeSurface(strings.Repeat("W", 200), false, -1); err == nil {
		t.Fatal("overflow notice accepted")
	}
}
