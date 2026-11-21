//go:build linux && mnet_enrollment_development && !mnet_os_process_launch

package osownership

func execUnprivilegedClientSource() error { return ErrExecutorUnreleased }
