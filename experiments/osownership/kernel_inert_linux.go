//go:build linux && mnet_enrollment_development && !mnet_os_process_launch

package osownership

import "os"

func proveNamespaceFD(int, Namespace) (*namespaceCustody, error) { return nil, ErrExecutorUnreleased }
func openExclusiveNonpersistentTUN(*namespaceCustody, string) (*os.File, error) {
	return nil, ErrExecutorUnreleased
}
