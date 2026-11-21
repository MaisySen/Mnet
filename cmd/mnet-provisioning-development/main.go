//go:build mnet_enrollment_development

// Disconnected stdin/stdout plan check only. No network/files/config install.
package main

import (
	"encoding/json"
	"fintex.au/mnet/internal/enrollmentprovisioning"
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "pass one secret-free disconnected plan through stdin, no options")
		os.Exit(2)
	}
	raw, e := io.ReadAll(io.LimitReader(os.Stdin, enrollmentprovisioning.MaxBytes+1))
	if e != nil {
		fmt.Fprintln(os.Stderr, "plan input unavailable")
		os.Exit(1)
	}
	report, e := enrollmentprovisioning.Verify(raw)
	if e != nil {
		fmt.Fprintln(os.Stderr, "Disconnected plan inconsistent. No authority or configuration was created.")
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(report) != nil {
		os.Exit(1)
	}
}
