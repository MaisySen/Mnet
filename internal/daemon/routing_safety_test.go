package daemon

import (
	"net/http"
	"net/http/httptest"
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
			switch role {
			case "selected-exit":
				d.state.UseExitNode = "gateway"
			case "advertised-exit":
				d.state.ExitNode = true
			case "mobile-subnet":
				d.state.AdvertiseRoutes = []string{"10.9.0.0/24"}
			}
			if err := d.Up(server.URL, "fixture", "node", ""); err == nil {
				t.Fatal("unsupported routing reported success")
			}
			if calls.Load() != 0 || d.dev != nil || d.Status().Status != "down" {
				t.Fatal("unsupported routing performed enrollment/device changes")
			}
		})
	}
}
