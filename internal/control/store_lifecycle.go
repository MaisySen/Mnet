package control

import (
	"errors"
	"fintex.au/mnet/internal/stateowner"
	"fmt"
)

var (
	ErrStoreClosed    = errors.New("store is closed")
	ErrStoreUncertain = errors.New("store publication uncertain; close and reopen required")
)

// Store retains its private API while native enrollment can share the same
// filesystem ownership implementation. Authority/rollback remain Store-owned.
type storeLease struct {
	owner        *stateowner.Lease
	path, parent string
}

func acquireStoreLease(path string) (*storeLease, string, error) {
	owner, canonical, err := stateowner.Acquire(path)
	if err != nil {
		return nil, "", err
	}
	return &storeLease{owner: owner, path: owner.LockPath(), parent: owner.Parent()}, canonical, nil
}
func (l *storeLease) check() error { return l.owner.Check() }

// healthLocked gates no-op mutations as well as authority reads. Ownership loss
// freezes the object; restoring the path does not revive it.
func (s *Store) healthLocked() error {
	if s.closed {
		return ErrStoreClosed
	}
	if s.fault != nil {
		return s.fault
	}
	if err := s.lease.check(); err != nil {
		s.freezeLocked(fmt.Errorf("%w: %v", ErrStoreUncertain, err))
		return s.fault
	}
	return nil
}

func (s *Store) freezeLocked(err error) {
	s.fault = err
	if s.registrarSources != nil {
		s.registrarSources.retired = true
		s.registrarSources.pool.Close()
	}
	s.relayIndex = nil
	for _, waiters := range s.watchers {
		for ch := range waiters {
			close(ch)
			delete(waiters, ch)
		}
	}
}

// Close permanently ends this object's authority before releasing ownership.
// It never unlinks the stable lock inode. Reopen loads a new disk image.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.freezeLocked(ErrStoreClosed)
	return s.lease.owner.Close()
}

// Health reports whether this owner can still serve authoritative state.
func (s *Store) Health() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthLocked()
}
