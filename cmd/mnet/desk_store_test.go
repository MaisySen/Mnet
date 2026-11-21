package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"fintex.au/mnet/internal/desk"
)

func writeStoreLayout(t *testing.T, dir, name string, layout desk.Layout) {
	t.Helper()
	data, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestListDeskStoreMissingDirCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	entries, err := listDeskStore(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("absent store = %v, %v; want empty list without error", entries, err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("browsing created the store directory")
	}
}

func TestListDeskStoreSummarizesDrafts(t *testing.T) {
	dir := t.TempDir()
	writeStoreLayout(t, dir, "b.json", starterDeskLayout(1024, 768))
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a draft"), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := listDeskStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "a.json" || entries[1].Name != "b.json" {
		t.Fatalf("entries = %+v, want sorted json drafts only", entries)
	}
	if !entries[0].Invalid || entries[0].Displays != 0 {
		t.Fatalf("unparseable draft must be marked invalid: %+v", entries[0])
	}
	if entries[1].Invalid || entries[1].Displays != 2 {
		t.Fatalf("valid draft summary wrong: %+v", entries[1])
	}
}

func TestNextDeskStoreNameSkipsUsed(t *testing.T) {
	entries := []deskStoreEntry{{Name: "arrangement-1.json"}, {Name: "arrangement-2.json"}}
	if got := nextDeskStoreName(entries); got != "arrangement-3.json" {
		t.Fatalf("next = %q, want arrangement-3.json", got)
	}
	if got := nextDeskStoreName(nil, "arrangement-1.json"); got != "arrangement-2.json" {
		t.Fatalf("next with extra = %q, want arrangement-2.json", got)
	}
}

func TestDeskArrangementsViewClampAndButtons(t *testing.T) {
	v := deskArrangementsView{}
	v.clamp()
	if v.sel != -1 || v.buttonEnabled(deskArrActInspect) || v.buttonEnabled(deskArrActEdit) {
		t.Fatalf("empty view must disable row actions: %+v", v)
	}
	if !v.buttonEnabled(deskArrActStarter) || !v.buttonEnabled(deskArrActClose) {
		t.Fatal("starter and close stay available on an empty store")
	}
	v = deskArrangementsView{pinned: "opened.json", entries: []deskStoreEntry{{Name: "arrangement-1.json", Displays: 2}}, sel: 99, focus: 99}
	v.clamp()
	if v.rows() != 2 || v.sel != 1 || v.focus != -1 {
		t.Fatalf("clamp = %+v", v)
	}
	if v.focusables() != 6 {
		t.Fatalf("focusables = %d, want 2 rows + 4 buttons", v.focusables())
	}
	if !v.buttonEnabled(deskArrActInspect) {
		t.Fatal("a selected row enables inspect")
	}
}

func TestListDeskStoreFlagsPlaceholderLabels(t *testing.T) {
	dir := t.TempDir()
	writeStoreLayout(t, dir, "starter.json", starterDeskLayout(1024, 768))
	custom := starterDeskLayout(1024, 768)
	custom.Displays[0].ID = "office-left"
	custom.Displays[0].Device = "desk-pc"
	custom.Displays[1].ID = "office-right"
	custom.Displays[1].Device = "laptop"
	writeStoreLayout(t, dir, "custom.json", custom)
	entries, err := listDeskStore(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	byName := map[string]deskStoreEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if !byName["starter.json"].Placeholders {
		t.Fatal("a starter-saved draft must be flagged as carrying placeholder labels")
	}
	if byName["custom.json"].Placeholders {
		t.Fatal("custom labels must not be flagged as placeholders")
	}
}
