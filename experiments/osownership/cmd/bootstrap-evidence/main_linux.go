//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence

package main

import (
	owner "fintex.au/mnet/experiments/osownership"
	"fmt"
	"os"
)

func main() {
	if e := owner.BootstrapEvidenceMain(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
