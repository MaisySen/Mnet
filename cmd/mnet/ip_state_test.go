//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/localapi"
)

func TestCLIIPStateProcess(t *testing.T) {
	if os.Getenv("MNET_IP_TEST_PROCESS") != "1" {
		return
	}
	os.Args = []string{"mnet", "--state", os.Getenv("MNET_IP_TEST_STATE"), "ip"}
	main()
	os.Exit(0)
}

func TestCLIIPRefusesRetainedAddressWhileNotUp(t *testing.T) {
	for _, state := range []string{"down", "preparing", "joining", "waiting-approval", "key-expired", "unknown", "up", "up-without-address"} {
		t.Run(state, func(t *testing.T) {
			dir, err := os.MkdirTemp("/tmp", "mnet-ip-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			token, err := localapi.EnsureToken(dir)
			if err != nil {
				t.Fatal(err)
			}
			network, address := daemon.SocketPath(dir)
			listener, err := net.Listen(network, address)
			if err != nil {
				t.Fatal(err)
			}
			server := &http.Server{Handler: localapi.RequireToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/localapi/status" {
					http.NotFound(w, r)
					return
				}
				status := daemon.Status{Status: state, IPv4: "100.64.0.7"}
				if state == "up-without-address" {
					status.Status, status.IPv4 = "up", ""
				}
				_ = json.NewEncoder(w).Encode(status)
			}))}
			go server.Serve(listener)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLIIPStateProcess$")
			command.Env = append(os.Environ(), "MNET_IP_TEST_PROCESS=1", "MNET_IP_TEST_STATE="+dir)
			output, err := command.CombinedOutput()
			if state == "up" {
				if err != nil || strings.TrimSpace(string(output)) != "100.64.0.7" {
					t.Fatal("current Up address unavailable", err, string(output))
				}
			} else {
				if err == nil || strings.Contains(string(output), "100.64.0.7") || !strings.Contains(string(output), "not up") {
					t.Error("inactive state returned retained mesh IP", err, string(output))
				}
			}
		})
	}
}
