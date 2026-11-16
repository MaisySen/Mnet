package daemon

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestEnrolledJoinTargetCannotChange(t *testing.T) {
	for _, mode := range []string{"up", "prepare"} {
		for _, change := range []string{"server", "network", "both"} {
			t.Run(mode+"/"+change, func(t *testing.T) {
				var originalCalls, otherCalls atomic.Int32
				original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					originalCalls.Add(1)
					if r.Header.Get("X-Node-ID") != "fixture-id" || r.Header.Get("X-Node-Token") != "fixture-token" {
						t.Error("saved credentials not sent to original destination")
					}
					w.WriteHeader(http.StatusForbidden)
				}))
				defer original.Close()
				other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherCalls.Add(1); w.WriteHeader(http.StatusForbidden) }))
				defer other.Close()
				dir := t.TempDir()
				s := &State{Server: original.URL, Network: "home", NodeName: "original"}
				if err := s.EnsureKey(dir); err != nil {
					t.Fatal(err)
				}
				s.NodeID = "fixture-id"
				s.NodeToken = "fixture-token"
				if err := s.Save(dir); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(statePath(dir))
				if err != nil {
					t.Fatal(err)
				}
				d, err := New(Config{StateDir: dir})
				if err != nil {
					t.Fatal(err)
				}
				server, network := original.URL, "home"
				if change == "server" || change == "both" {
					server = other.URL
				}
				if change == "network" || change == "both" {
					network = "different"
				}
				if mode == "up" {
					err = d.Up(server, network, "changed", "")
				} else {
					_, err = d.Prepare(server, network, "changed", "")
				}
				if err == nil || !strings.Contains(err.Error(), "enrolled") {
					t.Fatalf("target change not rejected: %v", err)
				}
				if originalCalls.Load() != 0 || otherCalls.Load() != 0 {
					t.Fatal("rejected target change contacted a server")
				}
				if d.Status().Status != "down" {
					t.Fatal("target rejection left daemon active")
				}
				if got := d.State(); !reflect.DeepEqual(got, *s) {
					t.Fatal("target rejection changed live identity")
				}
				after, err := os.ReadFile(statePath(dir))
				if err != nil || string(after) != string(before) {
					t.Fatal("target rejection changed saved identity")
				}
				// The original target remains usable, including a cosmetic trailing slash.
				// Our fixture deliberately denies enrollment; reaching it proves the
				// guard did not strand the saved identity or silently select a new one.
				if mode == "up" {
					err = d.Up(original.URL+"/", "home", "", "")
				} else {
					_, err = d.Prepare(original.URL+"/", "home", "", "")
				}
				if err == nil || originalCalls.Load() != 1 || otherCalls.Load() != 0 {
					t.Fatalf("original target retry was not preserved: %v", err)
				}
			})
		}
	}
}

func TestFreshJoinTargetMayBeSelected(t *testing.T) {
	for _, s := range []*State{{}, {Server: "old", Network: "old"}} {
		if err := s.validateJoinTarget("https://new.example", "new"); err != nil {
			t.Fatal(err)
		}
	}
	// Even an incomplete saved credential is not safe to transfer.
	for _, s := range []*State{{Server: "original", NodeID: "id"}, {Server: "original", NodeToken: "token"}} {
		if err := s.validateJoinTarget("different", ""); err == nil {
			t.Fatal("partial credential transferred")
		}
	}
}
