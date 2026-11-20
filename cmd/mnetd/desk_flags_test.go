package main

import (
	"flag"
	"io"
	"testing"
)

func TestDeskPathFlagsRejectEmptyAndRepeatedBeforeModeSelection(t *testing.T) {
	for _, name := range []string{"desk-run", "desk-inspect"} {
		for _, args := range [][]string{{"-" + name, ""}, {"-" + name + "="}, {"--" + name + "="}, {"-" + name, "one.json", "-" + name, "two.json"}, {"-" + name + "=one.json", "--" + name + "=two.json"}, {"-" + name, "", "-up"}} {
			fs := flag.NewFlagSet("fixture", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			selected := new(deskPathFlag)
			fs.Var(selected, name, "fixture")
			fs.Bool("up", false, "fixture")
			if err := fs.Parse(args); err == nil {
				t.Fatal("invalid explicit mode could start or select last file", args)
			}
		}
	}
}
func TestDeskPathFlagsPreserveExactOneFileAndAbsentLegacyMode(t *testing.T) {
	for _, args := range [][]string{nil, {"-desk-run", "a path/run.json"}, {"--desk-run=./relative.json"}} {
		fs := flag.NewFlagSet("fixture", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		selected := new(deskPathFlag)
		fs.Var(selected, "desk-run", "fixture")
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		if len(args) == 0 {
			if selected.seen || selected.path != "" {
				t.Fatal("absent mode selected")
			}
			continue
		}
		want := "a path/run.json"
		if len(args) == 1 {
			want = "./relative.json"
		}
		if !selected.seen || selected.path != want {
			t.Fatal(selected)
		}
		if selected.String() != "" {
			t.Fatal("flag help exposes selected private path")
		}
	}
}
