//go:build linux && mnet_enrollment_development && !mnet_os_process_launch

package osownership

import "context"

func (*originalHeldPacketResources) collectSourceDiagnostic(context.Context) (ownedSourceDiagnostic, error) {
	return ownedSourceDiagnostic{}, ErrExecutorUnreleased
}
