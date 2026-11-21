//go:build linux && mnet_enrollment_development

package osownership

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Static test-reference audit only, supplements build-time absence of real
// launch implementations. Unknown new test launch references require review.
func TestNoDirectRealEffectsFromUnitTests(t *testing.T) {
	files, e := filepath.Glob("*_test.go")
	if e != nil {
		t.Fatal(e)
	}
	denied := map[string]bool{"DevicePreflightExecutableMain": true, "spawnResidentMode": true, "spawnResidentPinnedMode": true, "spawnResidentPinnedSource": true, "startResidentClientSource": true, "runReadOnlyClientAndJoin": true, "execUnprivilegedClientSource": true, "residentBootstrapSource": true, "runDevicePreflightSource": true, "openExclusiveNonpersistentTUN": true, "installEvidenceClientFilter": true}
	for _, file := range files {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		f, e := parser.ParseFile(token.NewFileSet(), file, b, 0)
		if e != nil {
			t.Fatal(e)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := c.Fun.(*ast.Ident); ok && denied[id.Name] {
				t.Error("test references real effect function", file, id.Name)
			}
			if s, ok := c.Fun.(*ast.SelectorExpr); ok {
				if id, ok := s.X.(*ast.Ident); ok && (id.Name == "unix" || id.Name == "syscall" || id.Name == "exec") {
					allowed := map[string]bool{"Mkdev": true, "Major": true, "Minor": true}
					if !allowed[s.Sel.Name] {
						t.Error("test syscall/launch reference", file, id.Name, s.Sel.Name)
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
		t.Fatal("inert launch gained effects")
	}
}
