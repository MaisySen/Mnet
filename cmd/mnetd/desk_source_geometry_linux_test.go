//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Actual disposable-server root resize, never fake Setup data. xrandr owns its
// separate process/connection: calling randr.Init here would mutate XGB's global
// event decoder map while the active safety/capture connections are reading it.
func deskFixtureResizeSource(t *testing.T, c *xgb.Conn) func() {
	t.Helper()
	screen := xproto.Setup(c).DefaultScreen(c)
	original := fmt.Sprintf("%dx%d", screen.WidthInPixels, screen.HeightInPixels)
	run := func(args ...string) error {
		command := exec.Command("xrandr", args...)
		command.Env = os.Environ()
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("disposable root command: %w: %s", err, output)
		}
		return nil
	}
	var restored atomic.Bool
	restore := func() {
		if restored.Swap(true) {
			return
		}
		if err := run("--fb", original, "--output", "screen", "--mode", original); err != nil {
			t.Error(err)
		}
	}
	if err := run("--output", "screen", "--off", "--fb", "800x600"); err != nil {
		restore()
		t.Fatal(err)
	}
	geometry, err := xproto.GetGeometry(c, xproto.Drawable(screen.Root)).Reply()
	if err != nil || geometry.Width != 800 || geometry.Height != 600 {
		restore()
		t.Fatal("actual root did not resize", err)
	}
	return restore
}
