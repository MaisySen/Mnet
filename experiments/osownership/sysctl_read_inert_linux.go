//go:build linux && mnet_enrollment_development && !mnet_os_process_launch

package osownership

import "context"

func readPolicySysctlsSource(context.Context, *namespaceCustody, RawInventory) (SysctlSnapshot, error) {
	return nil, ErrExecutorUnreleased
}
