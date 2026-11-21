package control

import "testing"

// Every test Store owns a real OS lease. Reopens explicitly Close the old
// instance; cleanup also handles early test failures and Windows temp cleanup.
func openTestStore(t testing.TB, path string) (*Store, error) {
	t.Helper()
	s, err := Open(path)
	if s != nil {
		t.Cleanup(func() {
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	return s, err
}
