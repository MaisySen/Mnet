//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight && (!mnet_os_preflight_launch || !mnet_os_process_launch)

package osownership

import "context"

func runDevicePreflightSource(context.Context, *namespaceCustody) (DevicePreflightEvidence, error) {
	return DevicePreflightEvidence{}, ErrExecutorUnreleased
}
