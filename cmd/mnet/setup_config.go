package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"fintex.au/mnet/internal/enrollmentconfig"
)

// checkSetupConfig has no daemon/client/browser dependency. The explicitly named
// document is read only and never installed, saved or connected to enrollment.
func checkSetupConfig(args []string, w io.Writer) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New("usage: mnet setup-config-check FILE (offline check only)")
	}
	info, e := os.Lstat(args[0])
	if e != nil || !info.Mode().IsRegular() || info.Size() > enrollmentconfig.MaxBytes {
		return errors.New("setup configuration must be an existing regular file of at most 256 KiB")
	}
	f, e := os.Open(args[0])
	if e != nil {
		return errors.New("setup configuration could not be read")
	}
	defer f.Close()
	info, e = f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return errors.New("setup configuration must be a regular file")
	}
	raw, e := io.ReadAll(io.LimitReader(f, enrollmentconfig.MaxBytes+1))
	if e != nil {
		return errors.New("setup configuration could not be read")
	}
	if _, e = enrollmentconfig.Parse(raw); e != nil {
		return errors.New("setup configuration is invalid; use the exact documented schema, unique workspace/principal bindings and explicit HTTPS endpoints")
	}
	_, e = fmt.Fprintln(w, "Operator mapping syntax is valid. Nothing was installed or contacted.\nWorkspace/network existence, current permission, service authentication and protocol review remain unchecked.\nEnrollment and device pairing are not enabled.")
	return e
}
