//go:build linux && mnet_enrollment_development

package osownership

import (
	"errors"
	"os"
	"sync"
)

// *os.File Close is guarded once: never close a reused raw descriptor number.
type fileCustody struct {
	once    sync.Once
	mu      sync.Mutex
	file    *os.File
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
		if c.file != nil {
			c.err = errors.Join(c.err, c.file.Close())
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
