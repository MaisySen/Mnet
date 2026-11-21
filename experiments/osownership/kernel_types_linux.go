//go:build linux && mnet_enrollment_development

package osownership

import "errors"

var ErrExecutorUnreleased = errors.New("OS executor not released")

type namespaceCustody struct {
	fd       int
	identity Namespace
	threadID int
}

const kernelExecutionReleased = false
