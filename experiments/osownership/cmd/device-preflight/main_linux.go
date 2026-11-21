//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight && mnet_os_preflight_launch && mnet_os_process_launch

package main

import (
	"encoding/json"
	owner "fintex.au/mnet/experiments/osownership"
	"os"
)

func main() {
	if e := owner.DevicePreflightExecutableMain(os.Args[1:]); e != nil {
		_ = json.NewEncoder(os.Stderr).Encode(owner.DiagnosticForError(e))
		os.Exit(1)
	}
}
