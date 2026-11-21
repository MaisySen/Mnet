package control

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"fintex.au/mnet/internal/privatefs"
)

// Per-instance seam for exact failure injection around real Store files.
// Not exported and never configured from a request or environment variable.
type storeTemp interface {
	Name() string
	Write([]byte) (int, error)
	Sync() error
	Close() error
}
type storeFilesystem struct {
	createTemp func(string) (storeTemp, error)
	rename     func(string, string) error
	syncDir    func(string) error
}

func defaultStoreFilesystem() storeFilesystem {
	return storeFilesystem{
		createTemp: func(dir string) (storeTemp, error) { return os.CreateTemp(dir, ".mnet-state-*") },
		rename:     renameStoreFile, syncDir: syncStoreDir,
	}
}

// published is true once rename succeeded. A later failure must NEVER roll the
// live image back and continue serving authority from an older snapshot.
func (s *Store) publishLocked() (published bool, err error) { return s.publishGuardedLocked(nil) }

// Only the private registrar workflow supplies a final-boundary guard. Existing
// callers use nil and retain their exact legacy publication path.
func (s *Store) publishGuardedLocked(guard func() error) (published bool, err error) {
	if err := s.healthLocked(); err != nil {
		return false, err
	}
	if err := s.validateAdmissionFormat(); err != nil {
		return false, err
	}
	if err := s.validateRegistrarState(); err != nil {
		return false, err
	}
	b, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return false, err
	}
	if s.registrarIngress != nil && len(b) > s.registrarIngress.limits.MaxStateBytes {
		return false, errRegistrarEnrollment
	}
	if s.storageCodec != nil {
		b, err = s.storageCodec.encode(b)
		if err != nil {
			return false, err
		}
		if len(b) > s.storageCodec.maxBytes() || s.registrarIngress != nil && len(b) > s.registrarIngress.limits.MaxStateBytes {
			return false, fmt.Errorf("store: bounded format too large")
		}
	}
	// Reject path mutation, final links and parent replacement before publication.
	if s.path != s.ownedPath {
		return false, fmt.Errorf("store path changed while owned")
	}
	if info, err := os.Lstat(s.path); err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("store state must be a regular file")
		}
		if err := singleLink(s.path, info); err != nil {
			return false, err
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	dir := filepath.Dir(s.path)
	f, err := s.fs.createTemp(dir)
	if err != nil {
		return false, err
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if !closed {
			f.Close()
		}
		os.Remove(tmp)
	}()
	if err := privatefs.ProtectFile(tmp); err != nil {
		return false, err
	}
	if n, err := f.Write(b); err != nil {
		return false, err
	} else if n != len(b) {
		return false, io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		return false, err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return false, err
	}
	if err := s.lease.check(); err != nil {
		return false, err
	}
	tempInfo, statErr := os.Lstat(tmp)
	if statErr != nil {
		return false, statErr
	}
	oldInfo, oldErr := os.Lstat(s.path)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return false, oldErr
	}
	if guard != nil {
		if err := guard(); err != nil {
			return false, err
		}
	}
	if err := s.fs.rename(tmp, s.path); err != nil {
		// Even an error-returning publication operation may have replaced the
		// destination. Only classify rollback-safe when BOTH source and old target
		// are demonstrably unchanged; otherwise freeze conservatively.
		current, currentErr := os.Lstat(s.path)
		source, sourceErr := os.Lstat(tmp)
		unchangedTarget := oldErr == nil && currentErr == nil && os.SameFile(oldInfo, current)
		if os.IsNotExist(oldErr) && os.IsNotExist(currentErr) {
			unchangedTarget = true
		}
		unchangedSource := sourceErr == nil && os.SameFile(tempInfo, source)
		return !(unchangedTarget && unchangedSource), err
	}
	if err := s.fs.syncDir(dir); err != nil {
		return true, err
	}
	if err := s.lease.check(); err != nil {
		return true, err
	}
	if guard != nil {
		if err := guard(); err != nil {
			return true, err
		}
	}
	return true, nil
}

// saveLocked is retained for internal fixture/setup callers. Public mutations
// use commitLocked so prepublication failures undo their provisional changes.
func (s *Store) saveLocked() error {
	published, err := s.publishLocked()
	if err != nil && published {
		s.freezeLocked(fmt.Errorf("%w: %v", ErrStoreUncertain, err))
		return s.fault
	}
	return err
}

func (s *Store) commitLocked(rollback func()) error {
	published, err := s.publishLocked()
	if err == nil {
		return nil
	}
	if published {
		s.freezeLocked(fmt.Errorf("%w: %v", ErrStoreUncertain, err))
		return s.fault
	}
	rollback()
	return err
}
