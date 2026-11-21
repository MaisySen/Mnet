//go:build mnet_enrollment_development

package daemon

import (
	"context"
	"encoding/json"
	"errors"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDisposableEnrollmentRefusesBeforeCreatingNativeState(t *testing.T) {
	for _, mode := range []string{"no-marker", "insecure-marker", "insecure-policy", "insecure-limits", "remote-source", "remote-registrar", "long-timeout", "unknown-limit", "duplicate-limit", "case-alias-limit", "duplicate-timing", "trailing-limits", "symlink-root", "existing-native", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			policy := config.LocalConfigV2{Version: 2, Revision: "fixture", SourceOrigin: "https://127.0.0.1:30001", PrincipalAuthority: "https://principal.test", PrincipalNamespace: contract.PrincipalNamespace, IssuerLabel: "https://issuer.test/path", RegistrarOrigin: "https://127.0.0.1:30002", Audience: "fixture", ServiceID: "fixture-service", Bindings: []config.LocalBindingV2{{WorkspaceID: "workspace", NetworkID: "network", PrincipalIDs: []string{"fixture-person"}}}}
			if mode == "remote-source" {
				policy.SourceOrigin = "https://remote.test"
			}
			if mode == "remote-registrar" {
				policy.RegistrarOrigin = "https://remote.test"
			}
			raw, _ := json.Marshal(policy)
			limits := registrar.WorkflowLimits{Timeout: time.Second, MaxChallengeLifetime: time.Second, MaxReserveLifetime: time.Second, Timing: contract.FixtureTiming{MaxObservationAgeMS: 1000, MaxLeaseMS: 1000, ClockUncertaintyMS: 1}}
			if mode == "long-timeout" {
				limits.Timeout = 4 * time.Second
			}
			limitRaw, _ := json.Marshal(limits)
			if mode == "unknown-limit" {
				limitRaw = append([]byte(`{"unknown":1,`), limitRaw[1:]...)
			}
			if mode == "duplicate-limit" {
				limitRaw = append([]byte(`{"Timeout":1000000000,`), limitRaw[1:]...)
			}
			if mode == "case-alias-limit" {
				limitRaw = append([]byte(`{"timeout":1000000000,`), limitRaw[1:]...)
			}
			if mode == "duplicate-timing" {
				limitRaw = []byte(strings.Replace(string(limitRaw), `"Timing":{`, `"Timing":{"MaxLeaseMS":1000,`, 1))
			}
			if mode == "trailing-limits" {
				limitRaw = append(limitRaw, []byte(` {}`)...)
			}
			for name, body := range map[string][]byte{"DISPOSABLE-MNET-SETUP": []byte("mnet-disposable-enrollment-v1\n"), "enrollment-policy-v2.json": raw, "workflow-limits.json": limitRaw} {
				if mode == "no-marker" && name == "DISPOSABLE-MNET-SETUP" {
					continue
				}
				if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for modeName, file := range map[string]string{"insecure-marker": "DISPOSABLE-MNET-SETUP", "insecure-policy": "enrollment-policy-v2.json", "insecure-limits": "workflow-limits.json"} {
				if mode == modeName {
					if err := os.Chmod(filepath.Join(root, file), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			actual := root
			if mode == "symlink-root" {
				actual = filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(root, actual); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "existing-native" {
				if err := os.Mkdir(filepath.Join(root, "native"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "native", "sentinel"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			calls := 0
			if err := RunDisposableEnrollment(ctx, actual, func(DevelopmentEnrollmentEvent) error {
				calls++
				return errors.New("test refuses any actual browser launch")
			}); err == nil {
				t.Fatal("invalid development scope accepted")
			}
			if calls != 0 {
				t.Fatal("invalid scope emitted browser launch")
			}
			if mode == "existing-native" {
				data, err := os.ReadFile(filepath.Join(root, "native", "sentinel"))
				if err != nil || string(data) != "preserve" {
					t.Fatal("existing state changed")
				}
			} else if _, err := os.Stat(filepath.Join(root, "native")); !os.IsNotExist(err) {
				t.Fatal("invalid scope created native state", err)
			}
		})
	}
}
