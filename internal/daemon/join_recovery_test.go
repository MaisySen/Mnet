package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestJoinIncompleteCredentialPreservesIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"approved":true,"node_id":"replacement"}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := &State{Server: srv.URL, Network: "home", NodeName: "original"}
	if err := s.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	s.NodeID = "original-id"
	s.NodeToken = "original-token"
	s.AuthKey = "original-auth"
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(statePath(dir))
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.join(context.Background(), ""); err == nil {
		t.Fatal("malformed enrollment accepted")
	}
	if !reflect.DeepEqual(d.State(), *s) || d.client() != nil {
		t.Fatal("malformed enrollment changed live credentials or client")
	}
	after, err := os.ReadFile(statePath(dir))
	if err != nil || string(after) != string(before) {
		t.Fatal("malformed enrollment replaced saved identity")
	}
}

// Remote acceptance cannot be rolled back by a local disk error. Keep the
// received credential for an in-process retry, but do not publish a usable
// control client until that credential is saved. A process exit before the
// successful retry still needs operator recovery; this is not crash durability.
func TestJoinAcceptedCredentialSaveRetry(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		if n == 1 && r.Header.Get("X-Node-ID") != "" {
			t.Error("first join was not fresh")
		}
		if n == 2 && (r.Header.Get("X-Node-ID") != "accepted-id" || r.Header.Get("X-Node-Token") != "accepted-token") {
			t.Error("retry lost accepted credential")
		}
		json.NewEncoder(w).Encode(netmap.JoinResponse{NodeID: "accepted-id", NodeToken: "accepted-token", Approved: true})
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := &State{Server: srv.URL, Network: "home", NodeName: "original"}
	if err := s.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(statePath(dir))
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	d.cfg.StateDir = filepath.Join(statePath(dir), "invalid-child")
	err = d.join(context.Background(), "fixture-one-time-auth")
	if err == nil || !strings.Contains(err.Error(), "accepted") {
		t.Fatalf("missing enrollment persistence diagnostic: %v", err)
	}
	if d.State().NodeID != "accepted-id" || d.State().NodeToken != "accepted-token" || d.State().AuthKey != "fixture-one-time-auth" {
		t.Fatal("remote acceptance was discarded")
	}
	if d.client() != nil {
		t.Fatal("unsaved enrollment published a usable client")
	}
	after, err := os.ReadFile(statePath(dir))
	if err != nil || string(after) != string(before) {
		t.Fatal("failed enrollment save changed old disk state")
	}
	d.cfg.StateDir = dir
	if err := d.join(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || d.client() == nil || !reflect.DeepEqual(loaded, d.state) || loaded.PrivateKey != s.PrivateKey {
		t.Fatal("retry did not durably publish the accepted identity")
	}
}

func TestPendingApprovalURLNotLogged(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(netmap.JoinResponse{NodeID: "pending", NodeToken: "fixture-secret", ApprovalURL: "https://control.example/oidc/start?token=fixture-secret"})
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := &State{Server: srv.URL, Network: "home"}
	if err := s.EnsureKey(dir); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.join(ctx, "") }()
	deadline := time.Now().Add(10 * time.Second)
	for d.Status().Status != "waiting-approval" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if d.Status().Status != "waiting-approval" {
		cancel()
		<-done
		t.Fatal("approval wait did not start")
	}
	if !strings.Contains(d.Status().ApprovalURL, "fixture-secret") {
		t.Error("local approval action lost its URL")
	}
	cancel()
	<-done
	if strings.Contains(output.String(), "fixture-secret") || strings.Contains(output.String(), "token=") {
		t.Fatal("approval credential written to log")
	}
}
