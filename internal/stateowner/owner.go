// Package stateowner holds the shared local filesystem ownership primitive.
// It is not an authority store or journal. Callers retain their own serialization,
// strict data validation, publication/rollback/freeze and recovery contracts.
// Cooperative single-host writers only; not hostile-same-UID or remote FS safety.
package stateowner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Lease struct {
	file         *os.File
	path, parent string
	parentInfo   os.FileInfo
	closeOnce    sync.Once
	closeErr     error
}

// Acquire canonicalizes parent aliases and owns one stable sibling .lock inode.
// Existing shared-parent permissions are preserved. State file validation and
// recovery resync remain the caller's responsibility. No automatic stale unlock.
func Acquire(path string) (*Lease, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	parent := filepath.Dir(absolute)
	if err = makeParents(parent); err != nil {
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
	file, err := lockFile(lockPath)
	if err != nil {
		return nil, "", err
	}
	lease := &Lease{file: file, path: lockPath, parent: parent, parentInfo: info}
	if err = lease.Check(); err != nil {
		file.Close()
		return nil, "", err
	}
	return lease, path, nil
}
func (l *Lease) LockPath() string { return l.path }
func (l *Lease) Parent() string   { return l.parent }

// Check must be used before serving owned state or publishing a replacement.
// An error must permanently freeze the caller until Close and fresh acquisition;
// restoring a changed path must not resurrect the old authority object.
func (l *Lease) Check() error {
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
	return SingleLink(l.path, fileInfo)
}

// Close never unlinks the stable lock inode. New writers acquire a fresh owner.
func (l *Lease) Close() error {
	l.closeOnce.Do(func() { l.closeErr = l.file.Close() })
	return l.closeErr
}
func makeParents(path string) error {
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
	if err := makeParents(parent); err != nil {
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
	return SyncDir(parent)
}
