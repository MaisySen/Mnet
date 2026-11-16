package main

import (
	"encoding/json"
	"fmt"
	"io"

	"fintex.au/mnet/internal/daemon"
)

func statusJSONRequested(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, nil
	}
	return false, fmt.Errorf("usage: mnet status [--json]")
}

// Explicit JSON output is private local diagnostic data. Match local API
// fields without adding saved enrollment keys or control credentials.
func writeJSONStatus(out io.Writer, st *daemon.Status) error {
	return json.NewEncoder(out).Encode(st)
}
