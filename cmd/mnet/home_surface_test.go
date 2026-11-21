package main

import (
	"strings"
	"testing"
)

func TestHomeSurfaceAllRoutesAndNoticesFit(t *testing.T) {
	for _, notice := range homeNotices {
		for _, layout := range []bool{false, true} {
			for focus := -1; focus < 5; focus++ {
				im, err := renderHomeSurface(notice, layout, true, focus, false)
				if err != nil {
					t.Fatal(err)
				}
				if im.Bounds().Dx() != 900 || im.Bounds().Dy() != 560 {
					t.Fatal("home dimensions changed")
				}
			}
		}
	}
	if _, err := renderHomeSurface(strings.Repeat("W", 200), false, false, -1, false); err == nil {
		t.Fatal("overflow notice accepted")
	}
}

func TestHomeOriginIsOnlyExplicitStrictConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"--multies-server"}, {"--multies-server", ""},
		{"--multies-server", "https://multies.example.test", "--multies-server", "https://other.example.test"},
		{"--layout", "first", "--layout", "second"},
		{"--multies-server", "http://untrusted.example.test"},
		{"--multies-server", "https://multies.example.test/path"},
		{"--multies-server", "https://multies.example.test?redirect=x"},
		{"--multies-server", "https://multies.example.test\n"},
		{"--multies-server", "https://" + strings.Repeat("w", 76) + ".test"},
	} {
		if c, err := homeArgs(args); err == nil || c != (homeConfig{}) {
			t.Fatal("invalid config returned", args)
		}
	}
	for _, args := range [][]string{
		{"--multies-server", "https://multies.example.test", "--layout", "exact path.json"},
		{"--layout", "exact path.json", "--multies-server", "https://multies.example.test"},
	} {
		c, err := homeArgs(args)
		if err != nil || c.MultiesOrigin != "https://multies.example.test" || c.LayoutPath != "exact path.json" {
			t.Fatal("exact configuration lost", c, err)
		}
	}
	for _, configured := range []bool{false, true} {
		if _, err := renderHomeSurface("Approval review closed. Network setup is still not connected.", false, configured, 4, false); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHomeFifthControlDoesNotBorrowAKeyGesture(t *testing.T) {
	c := newDeviceControls()
	enabled := []bool{true, true, false, true, true}
	c.advanceAmong(true, enabled)
	if c.focus != 4 {
		t.Fatal("reverse Tab did not focus review")
	}
	c.pressKeyAmong(0xff0d, enabled)
	c.advanceAmong(true, enabled)
	if c.releaseKeyAmong(0xff0d, enabled) != -1 {
		t.Fatal("pending review answer crossed controls")
	}
	c.focus = 4
	c.pressKeyAmong(0x20, enabled)
	enabled[4] = false
	if c.releaseKeyAmong(0x20, enabled) != -1 {
		t.Fatal("disabled approval review accepted answer")
	}
	c.advanceAmong(false, nil)
	if c.focus != -1 {
		t.Fatal("empty controls retained focus")
	}
}

func TestControlNoSymbolReleaseCannotCompleteMousePress(t *testing.T) {
	c := newDeviceControls()
	enabled := [4]bool{true, false, false, true}
	c.focus = 0
	c.pressMouse(1, 30, 460, enabled)
	if c.releaseKey(0, enabled) != -1 {
		t.Fatal("NoSymbol release activated a mouse press")
	}
}

func TestHomeInfoPanelRendersAndFocusIsHeaderButton(t *testing.T) {
	for _, layout := range []bool{false, true} {
		for _, multies := range []bool{false, true} {
			im, err := renderHomeSurface("Choose a tool to begin.", layout, multies, 5, true)
			if err != nil {
				t.Fatal(err)
			}
			if im.Bounds().Dx() != 900 || im.Bounds().Dy() != 560 {
				t.Fatal("home dimensions changed")
			}
			// Focus underline on the header button while the panel is open.
			if p := im.RGBAAt(600, 21); p.R != 0x25 || p.G != 0x63 || p.B != 0xc7 {
				t.Fatal("info focus underline missing", p)
			}
			// Panel heading region is painted (dark heading ink).
			found := false
			for x := 38; x < 400 && !found; x += 2 {
				for y := 100; y < 116 && !found; y++ {
					p := im.RGBAAt(x, y)
					if p.R == 0x17 && p.G == 0x2b {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("info panel heading missing")
			}
			imOff, err := renderHomeSurface("Choose a tool to begin.", layout, multies, -1, false)
			if err != nil {
				t.Fatal(err)
			}
			if p := imOff.RGBAAt(600, 21); p.R != 0xff || p.G != 0xff {
				t.Fatal("info underline drawn without focus", p)
			}
			if p := imOff.RGBAAt(38, 120); p.R != 0xf4 && p.R != 0xff {
				t.Fatal("panel drawn while closed", p)
			}
		}
	}
}

func TestWrapHomeInfoBoundsLines(t *testing.T) {
	long := strings.Repeat("word ", 40)
	lines := wrapHomeInfo(long, 96)
	if len(lines) < 3 {
		t.Fatal("long explanation not wrapped")
	}
	for _, line := range lines {
		if len(line) > 96 {
			t.Fatal("wrapped line overflows", len(line))
		}
	}
	if got := wrapHomeInfo("short", 96); len(got) != 1 || got[0] != "short" {
		t.Fatal("short explanation changed", got)
	}
}
