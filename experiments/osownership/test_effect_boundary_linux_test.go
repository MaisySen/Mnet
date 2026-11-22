//go:build linux && mnet_enrollment_development

package osownership

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Static source tripwire, supplements absent live effects and compile conflict.
func TestNoDirectRealEffectsFromUnitTests(t *testing.T) {
	files, e := filepath.Glob("*_test.go")
	if e != nil {
		t.Fatal(e)
	}
	denied := map[string]bool{"DevicePreflightExecutableMain": true, "spawnResidentMode": true, "spawnResidentPinnedMode": true, "spawnResidentPinnedSource": true, "startResidentClientSource": true, "runReadOnlyClientAndJoin": true, "execUnprivilegedClientSource": true, "residentBootstrapSource": true, "runDevicePreflightSource": true, "openExclusiveNonpersistentTUN": true, "installEvidenceClientFilter": true, "emitClientBootstrapEvidence": true, "verifyClientSandboxSource": true, "rawCurrentNamespace": true, "numericCurrentNamespace": true, "pidfdAlive": true, "clientFDInventorySource": true, "rawReadBounded": true, "withResidentNamespace": true, "proveChildUserNamespace": true, "readProcessStartTicks": true}
	for _, name := range []string{"collectXfrmMetadataSource", "newEndpointParentSource", "spawnEndpointFilesSource", "spawnResidentPinnedModeFiles", "endpointResidentBootstrapSource", "newEndpointPipeWireSource", "endpointPipeIdentitySource", "newResidentEndpointAdmissionSource", "withInheritedEndpointResidentSource", "compareEndpointPreparedParentSource", "newOriginalEndpointReactor", "installedPolicyPassSource", "assessInstalledRowsSource", "checkInstalledOperandsSource", "PrivateEndpointNetworkOptions", "newDevelopmentEndpointNotifier", "ClaimPrivateEndpointBridge"} {
		denied[name] = true
	}
	forbiddenTypes := map[string]bool{"DevelopmentEndpointBridge": true, "developmentEndpointTun": true, "developmentEndpointDevice": true, "developmentEndpointEvent": true, "endpointParentSource": true, "endpointPipeWire": true, "residentEndpointAdmission": true, "originalEndpointReactor": true, "endpointOriginalLane": true, "residentChild": true, "namespaceCustody": true, "installedEndpointPolicyPass": true, "sourceRuntimeEvidence": true, "originalXfrmMetadataEvidence": true, "residentDevicePreflight": true, "residentNetlink": true}
	for _, file := range files {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		f, e := parser.ParseFile(token.NewFileSet(), file, b, 0)
		if e != nil {
			t.Fatal(e)
		}
		imported := map[string]string{}
		for _, im := range f.Imports {
			path, e := strconv.Unquote(im.Path.Value)
			if e != nil {
				t.Fatal(e)
			}
			name := filepath.Base(path)
			if im.Name != nil {
				name = im.Name.Name
			}
			imported[name] = path
			if path == "syscall" && im.Name != nil {
				t.Error("syscall exception forbids explicit aliases including dot/blank", file)
			}
			if path == "os/exec" {
				t.Error("forbidden test import path", file, path)
			}
			if path == "golang.org/x/sys/unix" && (name != "unix") {
				t.Error("unix helper exception requires unaliased import", file)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			// Ban references to concrete effectful types, including composites, new,
			// receiver casts and method expressions. Fake resident interface stays legal.
			if id, ok := n.(*ast.Ident); ok && forbiddenTypes[id.Name] {
				t.Error("forbidden concrete effect type reference", file, id.Name)
			}

			// Inspect EVERY syscall selector, not only direct calls. This also
			// rejects taking a callable address then invoking via local alias.
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && imported[id.Name] == "syscall" {
					switch sel.Sel.Name {
					case "EPERM", "EEXIST", "EBUSY", "ENODEV", "Errno":
					default:
						t.Error("syscall errno exception exceeded", file, sel.Sel.Name)
					}
				}
			}
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := c.Fun.(*ast.Ident); ok && denied[id.Name] {
				t.Error("test references real effect function", file, id.Name)
			}
			if s, ok := c.Fun.(*ast.SelectorExpr); ok {
				if s.Sel.Name == "collectOriginalXfrmLockedSource" || s.Sel.Name == "collectOriginalXfrmControlSource" || s.Sel.Name == "ClaimPrivateEndpointBridge" || s.Sel.Name == "PrivateEndpointNetworkOptions" || s.Sel.Name == "waitReadySource" || s.Sel.Name == "waitDataplaneSource" || s.Sel.Name == "StartPacketFlow" || s.Sel.Name == "installedPolicyPassSource" || s.Sel.Name == "assessInstalledRowsSource" || s.Sel.Name == "checkInstalledOperandsSource" {
					t.Error("test references private endpoint physical source", file, s.Sel.Name)
				}
				if id, ok := s.X.(*ast.Ident); ok && imported[id.Name] == "syscall" {
					if s.Sel.Name != "Errno" || len(c.Args) != 1 || c.Ellipsis.IsValid() {
						t.Error("only unary syscall.Errno conversion allowed", file)
					}
				}

				if s.Sel.Name == "Start" || s.Sel.Name == "Run" {
					// Transaction Start/Run is the one pure explicit exception in known tests.
					id, ok := s.X.(*ast.Ident)
					if !ok || (id.Name != "tx" && !(id.Name == "t" && s.Sel.Name == "Run")) {
						t.Error("forbidden potential command launch method", file, s.Sel.Name)
					}
				}
				if id, ok := s.X.(*ast.Ident); ok {
					path := imported[id.Name]
					if path == "os" && s.Sel.Name == "StartProcess" {
						t.Error("test process launch", file)
					}
					if path == "golang.org/x/sys/unix" {
						if s.Sel.Name != "Mkdev" && s.Sel.Name != "Major" && s.Sel.Name != "Minor" {
							t.Error("unix helper exception exceeded", file, s.Sel.Name)
						}
					}
				}
			}
			return true
		})
	}
	b, e := os.ReadFile("resident_launch_inert_linux.go")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "exec.Command(") || strings.Contains(string(b), ".Start(") {
		t.Fatal("inert launcher gained effects")
	}
}
