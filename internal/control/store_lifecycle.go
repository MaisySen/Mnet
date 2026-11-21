package control

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	ErrStoreClosed    = errors.New("store is closed")
	ErrStoreUncertain = errors.New("store publication uncertain; close and reopen required")
)

type storeLease struct {
	file       *os.File
	path       string
	parent     string
	parentInfo os.FileInfo
}

func acquireStoreLease(path string) (*storeLease, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	parent := filepath.Dir(absolute)
	if err = makeStoreParents(parent); err != nil {
		return nil, "", err
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, "", err
	}
	path = filepath.Join(parent, filepath.Base(absolute))
	info, err := os.Stat(parent)
	if err != nil {
		return nil, "", err
	}
	lockPath := path + ".lock"
	file, err := lockStoreFile(lockPath)
	if err != nil {
		return nil, "", err
	}
	lease := &storeLease{file: file, path: lockPath, parent: parent, parentInfo: info}
	if err = lease.check(); err != nil {
		file.Close()
		return nil, "", err
	}
	return lease, path, nil
}

func (l *storeLease) check() error {
	parent, err := os.Stat(l.parent)
	if err != nil {
		return fmt.Errorf("inspect store parent: %w", err)
	}
	if !os.SameFile(parent, l.parentInfo) {
		return errors.New("store parent replaced")
	}
	pathInfo, err := os.Lstat(l.path)
	if err != nil {
		return fmt.Errorf("inspect store ownership path: %w", err)
	}
	fileInfo, err := l.file.Stat()
	if err != nil {
		return err
	}
	if !pathInfo.Mode().IsRegular() || !os.SameFile(pathInfo, fileInfo) {
		return errors.New("store ownership file replaced")
	}
	return singleLink(l.path, fileInfo)
}

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
	return s.lease.file.Close()
}

// Preserve existing directory permissions. Newly created directory entries are
// synced in their parents before state can depend on them.
func makeStoreParents(path string) error {
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("store parent is not a directory")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return fmt.Errorf("store parent unavailable")
	}
	if err := makeStoreParents(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("store parent is not a directory")
	}
	return syncStoreDir(parent)
}

// Health reports whether this owner can still serve authoritative state.
func (s *Store) Health() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthLocked()
}
