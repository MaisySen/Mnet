//go:build linux && mnet_enrollment_development && !mnet_os_process_launch

package osownership

func disableOwnedIPv6(*namespaceCustody, *ownedCreation, Link, func() error) error {
	return ErrExecutorUnreleased
}
