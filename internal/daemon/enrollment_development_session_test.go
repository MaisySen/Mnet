//go:build mnet_enrollment_development

package daemon

import (
	"context"
	"encoding/base64"
	"fintex.au/mnet/internal/netmap"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"golang.zx2c4.com/wireguard/tun"
)

type developmentCountCloser struct{ calls atomic.Int32 }

func (c *developmentCountCloser) Close() error { c.calls.Add(1); return nil }

func TestDevelopmentNetworkRequiresExplicitBoundedInjection(t *testing.T) {
	valid := DevelopmentNetworkOptions{Limits: controlclient.RegistrarLimits{Timeout: time.Second, MaxConcurrent: 1, MaxRequests: 8}, Device: func(string) (tun.Device, error) { t.Fatal("refused options created a device"); return nil, nil }, Run: func(context.Context, *DevelopmentNetworkSession) error {
		t.Fatal("refused options ran session")
		return nil
	}}
	for _, name := range []string{"no-device", "no-run", "long-timeout", "no-timeout", "parallel-budget", "large-budget", "small-budget"} {
		t.Run(name, func(t *testing.T) {
			options := valid
			switch name {
			case "no-device":
				options.Device = nil
			case "no-run":
				options.Run = nil
			case "long-timeout":
				options.Limits.Timeout = 4 * time.Second
			case "no-timeout":
				options.Limits.Timeout = 0
			case "parallel-budget":
				options.Limits.MaxConcurrent = 2
			case "large-budget":
				options.Limits.MaxRequests = 17
			case "small-budget":
				options.Limits.MaxRequests = 3
			}
			if validDevelopmentNetworkOptions(&options) {
				t.Fatal("unsafe disposable session options admitted")
			}
			if err := RunDisposableEnrollmentNetwork(context.Background(), t.TempDir(), func(DevelopmentEnrollmentEvent) error { t.Fatal("refused options emitted event"); return nil }, options); err == nil {
				t.Fatal("invalid session admitted")
			}
		})
	}
}
func TestDevelopmentNetworkRejectsNonlocalEndpoints(t *testing.T) {
	for _, raw := range []string{"192.0.2.1:123", "[::1]:123", "localhost:123", "127.0.0.1:0", "https://127.0.0.1:123", "127.0.0.1"} {
		if developmentLoopbackEndpoint(raw) {
			t.Fatal("non-disposable endpoint admitted", raw)
		}
	}
	if !developmentLoopbackEndpoint("127.0.0.1:1234") {
		t.Fatal("owned loopback endpoint refused")
	}
}
func TestDevelopmentNetworkOwnClosesConsumersAndRejectsLateResources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &DevelopmentNetworkSession{ctx: ctx}
	c := &developmentCountCloser{}
	if err := s.Own(c); err != nil {
		t.Fatal(err)
	}
	cancel()
	s.closeResources()
	s.closeResources()
	if c.calls.Load() != 1 {
		t.Fatal("consumer was not closed exactly once")
	}
	late := &developmentCountCloser{}
	if err := s.Own(late); err == nil || late.calls.Load() != 1 {
		t.Fatal("retired session retained late resource")
	}
	if err := s.Own(io.Closer(nil)); err == nil {
		t.Fatal("nil consumer accepted")
	}
}

func TestDevelopmentNetworkMapValuesCannotBecomeIPCOrPeerSelection(t *testing.T) {
	expiry := time.Now().Add(time.Second)
	key := func(b byte) string {
		raw := make([]byte, 32)
		raw[0] = b
		return base64.StdEncoding.EncodeToString(raw)
	}
	original := netmap.Map{Network: netmap.Network{ID: "net"}, Self: netmap.Node{ID: "self", NetworkID: "net", IPv4: "100.64.0.1", PubKey: key(1), Approved: true, KeyExpiry: &expiry}, Peers: []netmap.Node{{ID: "peer", NetworkID: "net", Name: "peer", IPv4: "100.64.0.2", PubKey: key(2), Approved: true, KeyExpiry: &expiry, Endpoints: []string{"127.0.0.1:1234"}}}}
	if !validDevelopmentNetworkMap(&original) {
		t.Fatal("valid finite local map refused")
	}
	for _, name := range []string{"ipc-ip", "bad-key", "duplicate-key", "duplicate-ip", "unapproved", "missing-expiry", "remote-endpoint", "route", "exit-node", "wrong-network", "relay-url"} {
		t.Run(name, func(t *testing.T) {
			candidate := original
			candidate.Peers = append([]netmap.Node(nil), original.Peers...)
			switch name {
			case "ipc-ip":
				candidate.Peers[0].IPv4 = "100.64.0.2\nremove=true"
			case "bad-key":
				candidate.Peers[0].PubKey = "bad"
			case "duplicate-key":
				candidate.Peers = append(candidate.Peers, candidate.Peers[0])
				candidate.Peers[1].ID = "other"
				candidate.Peers[1].IPv4 = "100.64.0.3"
			case "duplicate-ip":
				candidate.Peers[0].IPv4 = candidate.Self.IPv4
			case "unapproved":
				candidate.Peers[0].Approved = false
			case "missing-expiry":
				candidate.Peers[0].KeyExpiry = nil
			case "remote-endpoint":
				candidate.Peers[0].Endpoints = []string{"192.0.2.1:123"}
			case "route":
				candidate.Peers[0].Routes = []string{"0.0.0.0/0"}
			case "exit-node":
				candidate.Peers[0].ExitNode = true
			case "wrong-network":
				candidate.Peers[0].NetworkID = "other"
			case "relay-url":
				candidate.RelayURL = "https://127.0.0.1:123"
			}
			if validDevelopmentNetworkMap(&candidate) {
				t.Fatal("unsafe map value admitted")
			}
		})
	}
}
func TestDevelopmentPeerScopeClosesOnceAndRejectsLateConsumer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	scope := &DevelopmentPeerScope{ctx: ctx, cancel: cancel}
	resource := &developmentCountCloser{}
	if err := scope.Own(resource); err != nil {
		t.Fatal(err)
	}
	scope.Close()
	scope.Close()
	if resource.calls.Load() != 1 || scope.Context().Err() == nil {
		t.Fatal("peer scope did not cancel and close exactly once")
	}
	late := &developmentCountCloser{}
	if scope.Own(late) == nil || late.calls.Load() != 1 {
		t.Fatal("retired peer admitted a consumer")
	}
	pin := SessionPeer{Name: "peer", IPv4: "100.64.0.2", PublicKey: "key"}
	if !developmentExactPeerPresent([]SessionPeer{pin}, pin) {
		t.Fatal("exact current peer refused")
	}
	for _, changed := range []SessionPeer{{Name: "changed", IPv4: pin.IPv4, PublicKey: pin.PublicKey}, {Name: pin.Name, IPv4: "100.64.0.3", PublicKey: pin.PublicKey}, {Name: pin.Name, IPv4: pin.IPv4, PublicKey: "changed"}} {
		if developmentExactPeerPresent([]SessionPeer{changed}, pin) {
			t.Fatal("changed peer admitted")
		}
	}
	if developmentExactPeerPresent([]SessionPeer{pin, pin}, pin) {
		t.Fatal("ambiguous duplicate peer admitted")
	}
}

func TestDevelopmentPeerOriginalDeadlineClosesWithoutAnotherMapCheck(t *testing.T) {
	scope := newDevelopmentPeerScope(context.Background(), SessionPeer{Name: "peer"}, time.Now().Add(60*time.Millisecond))
	defer scope.Close()
	resource := &developmentCountCloser{}
	if err := scope.Own(resource); err != nil {
		t.Fatal(err)
	}
	select {
	case <-scope.done:
	case <-time.After(time.Second):
		t.Fatal("original peer deadline left owner active")
	}
	if resource.calls.Load() != 1 || scope.Context().Err() == nil || scope.Reason() != "peer-expired" {
		t.Fatal("original deadline did not independently cancel/close peer scope")
	}
}
