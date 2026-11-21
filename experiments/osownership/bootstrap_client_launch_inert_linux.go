//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && !mnet_os_process_launch

package osownership

import "context"

func runReadOnlyClientAndJoin(context.Context, *namespaceCustody) (ClientBootstrapEvidence, error) {
	return ClientBootstrapEvidence{}, ErrExecutorUnreleased
}
