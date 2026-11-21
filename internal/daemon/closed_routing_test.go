package daemon

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClosedRoutingCannotRewriteSavedConfiguration(t *testing.T) {
	for _, reset := range []bool{false, true} {
		name := "replace"
		if reset {
			name = "reset"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			d, err := New(Config{StateDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if err := d.SetRouting(true, []string{"10.9.0.0/24"}, "saved-gateway"); err != nil {
				t.Fatal(err)
			}
			d.Close()
			before := d.State()
			path := filepath.Join(dir, "state.json")
			diskBefore, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if reset {
				err = d.SetRouting(false, nil, "")
			} else {
				err = d.SetRouting(false, []string{"10.8.0.0/24"}, "other-gateway")
			}
			if err == nil {
				t.Error("permanently closed node accepted routing mutation")
			}
			diskAfter, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !reflect.DeepEqual(before, d.State()) || !bytes.Equal(diskBefore, diskAfter) {
				t.Error("closed routing request rewrote memory or saved configuration")
			}
			fresh, err := New(Config{StateDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			if !reflect.DeepEqual(before, fresh.State()) {
				t.Error("later node inherited configuration from closed predecessor")
			}
		})
	}
}

func TestDownRoutingRemainsEditable(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Down()
	if err := d.SetRouting(true, []string{"10.9.0.0/24"}, "gateway"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetRouting(false, nil, ""); err != nil {
		t.Fatal(err)
	}
	d.Close()
	saved, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ExitNode || len(saved.AdvertiseRoutes) != 0 || saved.UseExitNode != "" {
		t.Fatal("ordinary Down/reset behavior changed")
	}
}
