package daemon

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestUnsupportedRoutingFailsBeforeControlOrDevice(t *testing.T) {
	for _, role := range []string{"selected-exit", "advertised-exit", "mobile-subnet"} {
		t.Run(role, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "unexpected", 500) }))
			defer server.Close()
			d, err := New(Config{StateDir: t.TempDir(), ManagedIface: role != "mobile-subnet"})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			var routes []string
			if role == "mobile-subnet" {
				routes = []string{"10.9.0.0/24"}
			}
			var exit string
			if role == "selected-exit" {
				exit = "gateway"
			}
			if err := d.SetRouting(role == "advertised-exit", routes, exit); err != nil {
				t.Fatal(err)
			}
			before := d.State()
			if err := d.Up(server.URL, "fixture", "node", ""); err == nil {
				t.Fatal("Up accepted unsupported routing")
			}
			if address, err := d.Prepare(server.URL, "fixture", "node", ""); err == nil || address != "" {
				t.Fatal("Prepare accepted unsupported routing")
			}
			if calls.Load() != 0 || d.dev != nil || d.Status().Status != "down" {
				t.Fatal("unsupported routing performed enrollment/device changes")
			}
			saved, err := LoadState(d.cfg.StateDir)
			if err != nil || !reflect.DeepEqual(before, d.State()) || !reflect.DeepEqual(before, *saved) {
				t.Fatal("unsupported routing changed identity or join settings", err)
			}
		})
	}
}
