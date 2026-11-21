//go:build linux && mnet_enrollment_development && !mnet_os_process_launch

package osownership

import (
	"context"
	"os/exec"
)

func spawnResidentMode(context.Context, string) (ResidentProcess, error) {
	return nil, ErrExecutorUnreleased
}
func spawnResidentPinnedSource(context.Context) (ResidentProcess, error) {
	return nil, ErrExecutorUnreleased
}
func spawnResidentPinnedMode(context.Context, string) (ResidentProcess, error) {
	return nil, ErrExecutorUnreleased
}
func startResidentClientSource(context.Context, *namespaceCustody) (*exec.Cmd, error) {
	return nil, ErrExecutorUnreleased
}

func parseSourceParentNamespace() (Namespace, error) { return Namespace{}, ErrExecutorUnreleased }
