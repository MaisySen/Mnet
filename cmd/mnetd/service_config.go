package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateSavedService is intentionally conservative for both service formats.
func validateSavedService(exe string, o options) error {
	// Conservative literal tokens prevent systemd specifier/environment/quoting
	// expansion. Unsupported paths fail rather than generating a different argv.
	safe := func(value string) bool {
		if value == "" {
			return false
		}
		for _, c := range value {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/._-", c)) {
				return false
			}
		}
		return true
	}
	if !filepath.IsAbs(exe) || !filepath.IsAbs(o.stateDir) || !safe(exe) || !safe(o.stateDir) || filepath.Clean(o.stateDir) == "/" || filepath.Clean(o.stateDir) != o.stateDir || filepath.Clean(exe) != exe || exe == "/" {
		return fmt.Errorf("service configuration requires absolute executable and dedicated state paths using only letters, digits, / . _ -")
	}
	if !safe(o.ifname) || strings.Contains(o.ifname, "/") || len(o.ifname) > 15 || o.ifname == "." || o.ifname == ".." {
		return fmt.Errorf("service configuration requires a literal interface name")
	}
	if o.authKey != "" || o.server != "" || o.network != "" || o.name != "" || o.advExit || o.advRoutes != "" || o.useExit != "" || o.sessHost || o.sessAuto || o.sessAllow != "" || o.sessRec != "" || o.sessProof || o.fakeTun || o.selfTest != "" || o.verbose || (o.proofDir != "" && o.proofDir != "/tmp/mnet-session-proof") {
		return fmt.Errorf("service configuration uses saved enrollment only; remove enrollment, routing, session and validation overrides (never put secrets in a unit)")
	}

	return nil
}
