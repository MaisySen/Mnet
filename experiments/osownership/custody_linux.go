//go:build linux && mnet_enrollment_development

package osownership

import (
	"os"
	"sync"
)

// *os.File Close is guarded once: never close a reused raw descriptor number.
type fileCustody struct {
	once sync.Once
	file *os.File
	err  error
}

func (c *fileCustody) Close() error {
	c.once.Do(func() {
		if c.file != nil {
			c.err = c.file.Close()
		}
	})
	return c.err
}
