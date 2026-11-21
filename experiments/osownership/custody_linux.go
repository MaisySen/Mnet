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
	file    *os.File
	err     error
	lineage *heldPacketLedger
	closed  bool
}

func (c *fileCustody) Close() error {
	c.once.Do(func() {
		c.closed = true
		if c.lineage != nil {
			c.err = errors.Join(c.err, c.lineage.invalidate("close-intent"))
		}
		if c.file != nil {
			c.err = errors.Join(c.err, c.file.Close())
		}
	})
	return c.err
}
