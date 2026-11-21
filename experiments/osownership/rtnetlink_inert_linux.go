//go:build linux && mnet_enrollment_development && !mnet_os_process_launch

package osownership

import "context"

func (n *namespaceCustody) checkCurrent() error { return ErrExecutorUnreleased }

type residentNetlink struct{ ns *namespaceCustody }

func (residentNetlink) Request(context.Context, uint16, []byte, bool) ([][]byte, error) {
	return nil, ErrExecutorUnreleased
}
func (residentNetlink) applyExact(context.Context, ExactMutation) error { return ErrExecutorUnreleased }
