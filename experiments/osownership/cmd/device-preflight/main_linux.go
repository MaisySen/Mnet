//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight

package main

import (
	"encoding/json"
	owner "fintex.au/mnet/experiments/osownership"
	"os"
)

func main() {
	if e := owner.DevicePreflightMain(os.Args[1:]); e != nil {
		_ = json.NewEncoder(os.Stderr).Encode(owner.DiagnosticForError(e))
		os.Exit(1)
	}
}
