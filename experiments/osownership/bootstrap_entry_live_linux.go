//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_process_launch

package osownership

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"
)

func BootstrapEvidenceMain(args []string) error {
	if len(args) != 1 {
		return errors.New("exactly one fixed evidence mode required")
	}
	switch args[0] {
	case "--bootstrap-readonly":
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		outer, e := numericCurrentNamespace()
		if e != nil {
			return e
		}
		v, runErr := SuperviseResident(ctx, residentSpawner{})
		if runErr == nil {
			runErr = validateBootstrapEvidence(v.Bootstrap, outer)
		}
		after, afterErr := numericCurrentNamespace()
		runErr = errors.Join(runErr, afterErr)
		unchanged := afterErr == nil && after == outer
		if !unchanged {
			runErr = errors.Join(runErr, ErrConflict)
		}
		result := struct {
			Version        int       `json:"version"`
			Outcome        string    `json:"outcome"`
			OuterInitial   Namespace `json:"outer_initial"`
			OuterFinal     Namespace `json:"outer_final"`
			OuterUnchanged bool      `json:"outer_unchanged"`
			Verdict        Verdict   `json:"verdict"`
			Error          string    `json:"error,omitempty"`
		}{Version: 1, Outcome: "pass", OuterInitial: outer, OuterFinal: after, OuterUnchanged: unchanged, Verdict: v}
		if runErr != nil {
			result.Outcome = "fail"
			result.Error = runErr.Error()
		}
		return errors.Join(runErr, json.NewEncoder(os.Stdout).Encode(result))
	case endpointResidentMode:
		return endpointResidentBootstrapSource(context.Background())
	case "--osownership-resident-no-tun-source":
		return residentBootstrapSource(context.Background(), runReadOnlyBootstrapEvidence)
	case "--osownership-client-capdrop-bootstrap-source":
		return execUnprivilegedClientSource()
	case "--osownership-client-unprivileged-source":
		return emitClientBootstrapEvidence()
	}
	return errors.New("unknown fixed evidence mode")
}
