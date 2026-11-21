package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fintex.au/mnet/internal/desk"
)

// The home Shared Desk journey keeps offline arrangement drafts in one
// private store directory under the explicit state directory. A draft is
// local file data only: never pairing, identity, approval or input.
const (
	deskStoreDirName = "desk-layouts"
	deskStoreLimit   = 64
)

var errStoreUnavailable = errors.New("arrangement store unavailable")

func deskStoreDir(stateDir string) string {
	return filepath.Join(stateDir, deskStoreDirName)
}

type deskStoreEntry struct {
	Name     string // file base name inside the store directory
	Displays int    // validated display count; 0 when the file is not a valid draft
	Invalid  bool   // true when the file is present but not a valid arrangement
}

// listDeskStore is read-only. A missing store directory is an empty list,
// never an error: browsing drafts must not create anything.
func listDeskStore(dir string) ([]deskStoreEntry, error) {
	d, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errStoreUnavailable
	}
	defer d.Close()
	names, err := d.Readdirnames(deskStoreLimit + 1)
	if err != nil {
		return nil, errStoreUnavailable
	}
	sort.Strings(names)
	var entries []deskStoreEntry
	for _, name := range names {
		if len(entries) == deskStoreLimit {
			break
		}
		if !strings.HasSuffix(name, ".json") || strings.ContainsRune(name, '/') {
			continue
		}
		entry := deskStoreEntry{Name: name}
		if f, err := os.Open(filepath.Join(dir, name)); err == nil {
			layout, decErr := desk.Decode(f)
			f.Close()
			if decErr == nil {
				if _, compErr := desk.Compile(layout); compErr == nil {
					entry.Displays = len(layout.Displays)
				} else {
					entry.Invalid = true
				}
			} else {
				entry.Invalid = true
			}
		} else {
			entry.Invalid = true
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// nextDeskStoreName picks the first free arrangement-N.json. Saving stays
// create-only, so a name taken after listing is refused by the save path
// instead of replaced.
func nextDeskStoreName(entries []deskStoreEntry, extra ...string) string {
	used := map[string]bool{}
	for _, e := range entries {
		used[e.Name] = true
	}
	for _, name := range extra {
		used[name] = true
	}
	for n := 1; n < 10000; n++ {
		name := "arrangement-" + strconv.Itoa(n) + ".json"
		if !used[name] {
			return name
		}
	}
	return "arrangement-9999.json"
}
