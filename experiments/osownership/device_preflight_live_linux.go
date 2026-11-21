//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight && mnet_os_preflight_launch && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/json"
	"os"
)

type devicePreflightSpawner struct{}

func (devicePreflightSpawner) Spawn(ctx context.Context) (ResidentProcess, error) {
	return spawnResidentMode(ctx, "--osownership-resident-device-open-preflight")
}

// Executable binding is NOT compiled into ordinary unit/mutation builds.
func DevicePreflightExecutableMain(args []string) error {
	return devicePreflightDispatch(args, devicePreflightEntryOps{
		uid: os.Getuid, namespace: numericCurrentNamespace,
		supervise: func(ctx context.Context) (Verdict, error) { return SuperviseResident(ctx, devicePreflightSpawner{}) },
		resident:  func(ctx context.Context) error { return residentBootstrapSource(ctx, runDevicePreflightVerdict) },
		output:    func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) },
	})
}
