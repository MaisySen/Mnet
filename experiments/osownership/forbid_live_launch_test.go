//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

// Deliberate duplicate declaration with production-only file: unit/mutation
// binaries cannot link the real process launch implementation, even accidentally.
func unitTestLiveLaunchBuildConflict() {}
