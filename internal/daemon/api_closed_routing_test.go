//go:build !windows

package daemon

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/localapi"
)

func TestLocalAPIClosedUpCannotRewriteRouting(t *testing.T) {
	for _, request := range []struct{ name, body string }{
		{"reset", `{"reset_routing":true}`},
		{"replace", `{"advertise_routes":["10.8.0.0/24"],"exit_node":"other-gateway"}`},
	} {
		t.Run(request.name, func(t *testing.T) {
			dir := localAPITestDir(t)
			d, err := New(Config{StateDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if err := d.SetRouting(true, []string{"10.9.0.0/24"}, "saved-gateway"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := make(chan error, 1)
			done := make(chan error, 1)
			go func() { done <- d.ServeLocalAPIContext(ctx, ready) }()
			if err := <-ready; err != nil {
				t.Fatal(err)
			}
			defer func() {
				cancel()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			network, addr := SocketPath(dir)
			base := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			}}
			defer base.CloseIdleConnections()
			client := &http.Client{Transport: localapi.Transport{StateDir: dir, Base: base}, Timeout: 5 * time.Second}
			d.Close()
			before := d.State()
			diskBefore, err := os.ReadFile(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Post("http://mnetd/localapi/up", "application/json", strings.NewReader(request.body))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatal("async acknowledgement changed", response.StatusCode)
			}
			deadline := time.Now().Add(5 * time.Second)
			for !strings.Contains(d.Status().LastError, "node is closed") && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if !strings.Contains(d.Status().LastError, "node is closed") {
				t.Fatal("worker did not reach closed refusal")
			}
			diskAfter, err := os.ReadFile(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, d.State()) || !bytes.Equal(diskBefore, diskAfter) {
				t.Fatal("authenticated delayed Up rewrote closed node routing")
			}
		})
	}
}
