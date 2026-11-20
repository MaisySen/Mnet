package main

import "errors"

// Flag parsing must distinguish an absent engineering mode from an explicit
// empty/repeated selector. Neither can fall through to daemon startup or choose
// a different last file silently. A file remains untrusted data, not authority.
type deskPathFlag struct {
	path string
	seen bool
}

func (f *deskPathFlag) String() string { return "" }
func (f *deskPathFlag) Set(path string) error {
	if f.seen {
		return errors.New("Shared Desk file selector cannot be repeated")
	}
	f.seen = true
	if path == "" {
		return errors.New("Shared Desk file selector cannot be empty; no daemon fallback")
	}
	f.path = path
	return nil
}
