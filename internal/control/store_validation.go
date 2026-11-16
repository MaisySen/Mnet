package control

import "fmt"

// Missing map fields in older object-format state retain Open's initialized
// defaults. Explicit null maps/records are corruption, not an empty network:
// refuse startup without overwriting the source or silently dropping records.
// This is a shape check, not full semantic validation or automatic repair.
func (s *Store) validateLoadedShape() error {
	if s.data.Networks == nil || s.data.Nodes == nil || s.data.AuthKeys == nil || s.data.NodeTokens == nil || s.data.NextIP == nil {
		return fmt.Errorf("store: null required state map; file was not changed")
	}
	for _, n := range s.data.Networks {
		if n == nil {
			return fmt.Errorf("store: null network record; file was not changed")
		}
	}
	for _, n := range s.data.Nodes {
		if n == nil {
			return fmt.Errorf("store: null node record; file was not changed")
		}
	}
	for _, k := range s.data.AuthKeys {
		if k == nil {
			return fmt.Errorf("store: null enrollment key record; file was not changed")
		}
	}
	return nil
}
