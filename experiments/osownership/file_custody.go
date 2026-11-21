//go:build mnet_enrollment_development

package osownership

import (
	"errors"
	"os"
	"sync"
)

type originalCloser interface{ Close() error }

// *os.File Close is guarded once: never close a reused raw descriptor number.
type fileCustody struct {
	once    sync.Once
	mu      sync.Mutex
	file    *os.File
	closer  originalCloser
	err     error
	lineage *heldPacketLedger
	closed  bool
}

func (c *fileCustody) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		lineage := c.lineage
		c.mu.Unlock()
		if lineage != nil {
			c.err = errors.Join(c.err, lineage.invalidate("close-intent"))
		}
		closer := c.closer
		if closer == nil && c.file != nil {
			closer = c.file
		}
		if closer == nil {
			c.err = errors.Join(c.err, ErrConflict)
		} else {
			c.err = errors.Join(c.err, closer.Close())
		}
	})
	return c.err
}

func (c *fileCustody) attachLineage(l *heldPacketLedger) error {
	if c == nil || l == nil {
		return ErrConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.file == nil || c.lineage != nil {
		return ErrConflict
	}
	c.lineage = l
	return nil
}
func (c *fileCustody) effectLineage() (*heldPacketLedger, error) {
	if c == nil {
		return nil, ErrConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.file == nil || c.lineage == nil {
		return nil, ErrConflict
	}
	return c.lineage, nil
}
