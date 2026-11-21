//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && !mnet_os_process_launch

package osownership

func BootstrapEvidenceMain([]string) error { return ErrExecutorUnreleased }
