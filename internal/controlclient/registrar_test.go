package controlclient

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/netmap"
)

func registrarFixture(t *testing.T, origin string) contract.NativeCredentialDocument {
	t.Helper()
	raw, err := os.ReadFile("../enrollmentcontract/testdata/native-credential-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := contract.DecodeNativeCredentialDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	d.Challenge.Binding.RegistrarOrigin = origin
	d.Challenge.Binding.CheckedAtMS = now.Add(-time.Second).UnixMilli()
	d.Challenge.Binding.AccessUntilMS = now.Add(time.Minute).UnixMilli()
	d.Challenge.Binding.ReserveDeadlineMS = now.Add(2 * time.Minute).UnixMilli()
	d.Challenge.ExpiresAtMS = now.Add(30 * time.Second).UnixMilli()
	r, err := contract.ReceiptV2ForChallenge(d.Challenge, d.Receipt.NodeID, d.Receipt.ReceiptID, now.Add(-time.Millisecond).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	d.Receipt = r
	d.ReceiptDigest = contract.ReceiptV2Digest(r)
	return d
}
func registrarMap(d contract.NativeCredentialDocument) *netmap.Map {
	end := time.UnixMilli(d.Receipt.AccessExpiresAtMS).UTC()
	return &netmap.Map{Version: 1, Network: netmap.Network{ID: d.Receipt.NetworkID, Name: "fixture"}, Self: netmap.Node{ID: d.Receipt.NodeID, NetworkID: d.Receipt.NetworkID, PubKey: d.Receipt.DevicePublicKey, Approved: true, IPv4: "100.64.0.1", KeyExpiry: &end}}
}
func registrarTrusted(t *testing.T, s *httptest.Server, d contract.NativeCredentialDocument, limits RegistrarLimits) *RegistrarClient {
	t.Helper()
	c, err := NewRegistrarClient(s.URL, d, limits)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	c.transport.TLSClientConfig.RootCAs = roots
	t.Cleanup(c.Close)
	return c
}
func registrarHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
}
func TestRegistrarDedicatedCurrentWireAndNoLegacyRejoin(t *testing.T) {
	var doc contract.NativeCredentialDocument
	var calls atomic.Int64
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != registrarNodeScheme+doc.Secret || r.ProtoMajor != 1 {
			t.Error("unexpected request")
		}
		for _, name := range []string{"X-Node-ID", "X-Node-Token", "Cookie", "Origin", "Referer"} {
			if r.Header.Get(name) != "" {
				t.Error("ambient/legacy header")
			}
		}
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte(doc.Secret)) {
			t.Error("body contains native secret")
		}
		var payload registrarNodeRequest
		if json.Unmarshal(body, &payload) != nil || payload.Version != 1 || payload.Receipt != doc.Receipt || payload.ReceiptDigest != doc.ReceiptDigest {
			t.Error("qualified receipt scope missing")
		}
		registrarHeaders(w)
		w.Header().Set("Set-Cookie", "discard=1; Secure")
		switch r.URL.Path {
		case registrarNodePath + "map":
			json.NewEncoder(w).Encode(registrarMapResponse{1, doc.ReceiptDigest, registrarMap(doc)})
		case registrarNodePath + "endpoints":
			if len(payload.Endpoints) != 2 {
				t.Error("endpoints changed")
			}
			json.NewEncoder(w).Encode(registrarEndpointsResponse{1, doc.ReceiptDigest, true})
		default:
			t.Error("legacy/arbitrary path")
		}
	}))
	defer s.Close()
	doc = registrarFixture(t, s.URL)
	c := registrarTrusted(t, s, doc, RegistrarLimits{time.Second, 1, 3})
	if c.http.Jar != nil || c.transport.Proxy != nil || c.transport.TLSClientConfig.InsecureSkipVerify || !c.transport.DisableKeepAlives || c.transport.ForceAttemptHTTP2 || len(c.transport.TLSNextProto) != 0 {
		t.Fatal("ambient transport")
	}
	got, err := c.CurrentMap(context.Background())
	if err != nil || got.Self.ID != doc.Receipt.NodeID {
		t.Fatal("own map", err)
	}
	if err := c.RegistrarEndpoints(context.Background(), []string{"192.0.2.1:51820", "[2001:db8::1]:51820"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CurrentMap(context.Background()); err != nil {
		t.Fatal("allowed third request", err)
	}
	if _, err := c.CurrentMap(context.Background()); err != ErrRegistrarInvalid || calls.Load() != 3 {
		t.Fatal("request budget refunded/reset")
	}
	for _, f := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(f, c), doc.Secret) {
			t.Fatal("secret exposed")
		}
	}
}
func TestRegistrarCurrentDeniedPermanentlyRetiresOwnerAndNotLegacyUnauthorized(t *testing.T) {
	var calls atomic.Int64
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(401)
		w.Write([]byte("upstream private body"))
	}))
	defer s.Close()
	doc := registrarFixture(t, s.URL)
	c := registrarTrusted(t, s, doc, RegistrarLimits{time.Second, 1, 4})
	copyClient := *c
	if _, err := c.CurrentMap(context.Background()); err != ErrRegistrarDenied || errors.Is(err, ErrUnauthorized) || strings.Contains(err.Error(), "upstream") {
		t.Fatal("wrong denied semantics", err)
	}
	if err := copyClient.RegistrarEndpoints(context.Background(), nil); err != ErrRegistrarDenied || calls.Load() != 1 {
		t.Fatal("denied owner retried")
	}
	if c.document != (contract.NativeCredentialDocument{}) {
		t.Fatal("denied owner retained reachable secret")
	}
}
func TestRegistrarCurrentMapRejectsCrossEpochAndBadGrammar(t *testing.T) {
	for _, kind := range []string{"node", "network", "key", "receipt", "expiry", "unapproved", "relay", "routes", "null-map", "duplicate", "case", "missing", "fraction", "unknown", "status", "cache", "encoding", "redirect", "size"} {
		t.Run(kind, func(t *testing.T) {
			var doc contract.NativeCredentialDocument
			var calls, sinkCalls atomic.Int64
			sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkCalls.Add(1) }))
			defer sink.Close()
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				registrarHeaders(w)
				m := registrarMap(doc)
				response := registrarMapResponse{1, doc.ReceiptDigest, m}
				switch kind {
				case "node":
					m.Self.ID += "x"
				case "network":
					m.Network.ID += "x"
				case "key":
					m.Self.PubKey += "x"
				case "receipt":
					response.ReceiptDigest += "x"
				case "expiry":
					end := m.Self.KeyExpiry.Add(time.Second)
					m.Self.KeyExpiry = &end
				case "unapproved":
					m.Self.Approved = false
				case "relay":
					m.RelayURL = "wss://arbitrary.invalid/?token=private"
				case "routes":
					m.Self.Routes = []string{"0.0.0.0/0"}
				case "null-map":
					response.Map = nil
				case "status":
					w.WriteHeader(500)
					return
				case "cache":
					w.Header().Set("Cache-Control", "public")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "redirect":
					w.Header().Set("Location", sink.URL)
					w.WriteHeader(307)
					return
				case "size":
					w.Write(bytes.Repeat([]byte(" "), (4<<20)+1))
					return
				}
				raw, _ := json.Marshal(response)
				switch kind {
				case "duplicate":
					raw = []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1))
				case "case":
					raw = []byte(strings.Replace(string(raw), `"version":1`, `"Version":1`, 1))
				case "missing":
					raw = []byte(strings.Replace(string(raw), `"version":1,`, "", 1))
				case "fraction":
					raw = []byte(strings.Replace(string(raw), `"version":1`, `"version":1.0`, 1))
				case "unknown":
					raw = []byte(strings.Replace(string(raw), `"approved":true`, `"approved":true,"arbitrary":0`, 1))
				}
				w.Write(raw)
			}))
			defer s.Close()
			doc = registrarFixture(t, s.URL)
			c := registrarTrusted(t, s, doc, RegistrarLimits{time.Second, 1, 1})
			if m, err := c.CurrentMap(context.Background()); err != ErrRegistrarUnavailable || m != nil {
				t.Fatal("wrong map accepted", kind, err)
			}
			if _, err := c.CurrentMap(context.Background()); err != ErrRegistrarInvalid || calls.Load() != 1 || sinkCalls.Load() != 0 {
				t.Fatal("failed call retried/redirected")
			}
		})
	}
}
func TestRegistrarCurrentConcurrencyCloseAndInvalidEndpoints(t *testing.T) {
	var doc contract.NativeCredentialDocument
	var calls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		registrarHeaders(w)
		json.NewEncoder(w).Encode(registrarMapResponse{1, doc.ReceiptDigest, registrarMap(doc)})
	}))
	defer s.Close()
	defer close(release)
	doc = registrarFixture(t, s.URL)
	c := registrarTrusted(t, s, doc, RegistrarLimits{time.Second * 2, 1, 2})
	copyClient := *c
	for _, eps := range [][]string{{"dns.invalid:1"}, {"192.0.2.1:0"}, {"[fe80::1%zone]:1"}, {"[::ffff:192.0.2.1]:1"}} {
		if err := c.RegistrarEndpoints(context.Background(), eps); err != ErrRegistrarInvalid {
			t.Fatal("bad endpoint admitted")
		}
	}
	result := make(chan error, 1)
	go func() { _, err := c.CurrentMap(context.Background()); result <- err }()
	<-started
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := copyClient.CurrentMap(context.Background()); err != ErrRegistrarInvalid {
				t.Error("concurrency cap ignored")
			}
		}()
	}
	wg.Wait()
	copyClient.Close()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Close returned late map")
		}
	case <-time.After(time.Second):
		t.Fatal("Close failed to cancel")
	}
	if calls.Load() != 1 || c.document != (contract.NativeCredentialDocument{}) {
		t.Fatal("unexpected send/secret retention")
	}
}
func TestRegistrarCurrentExpiryAndConstructorScope(t *testing.T) {
	var doc contract.NativeCredentialDocument
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		time.Sleep(180 * time.Millisecond)
		registrarHeaders(w)
		json.NewEncoder(w).Encode(registrarMapResponse{1, doc.ReceiptDigest, registrarMap(doc)})
	}))
	defer s.Close()
	doc = registrarFixture(t, s.URL)
	doc.Challenge.Binding.AccessUntilMS = time.Now().Add(100 * time.Millisecond).UnixMilli()
	doc.Challenge.ExpiresAtMS = doc.Challenge.Binding.AccessUntilMS
	doc.Receipt, _ = contract.ReceiptV2ForChallenge(doc.Challenge, doc.Receipt.NodeID, doc.Receipt.ReceiptID, doc.Receipt.CommittedAtMS)
	doc.ReceiptDigest = contract.ReceiptV2Digest(doc.Receipt)
	c := registrarTrusted(t, s, doc, RegistrarLimits{time.Second, 1, 2})
	if m, err := c.CurrentMap(context.Background()); err == nil || m != nil {
		t.Fatal("expired request returned map")
	}
	if _, err := NewRegistrarClient(s.URL+"/other", doc, RegistrarLimits{time.Second, 1, 2}); err != ErrRegistrarInvalid {
		t.Fatal("other destination accepted")
	}
}

func TestRegistrarCurrentEndpointAcknowledgementAndTLSRefusal(t *testing.T) {
	for _, kind := range []string{"wrong-receipt", "not-updated", "wrong-purpose", "untrusted-tls"} {
		t.Run(kind, func(t *testing.T) {
			var doc contract.NativeCredentialDocument
			var calls atomic.Int64
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				registrarHeaders(w)
				response := registrarEndpointsResponse{1, doc.ReceiptDigest, true}
				switch kind {
				case "wrong-receipt":
					response.ReceiptDigest += "x"
				case "not-updated":
					response.Updated = false
				case "wrong-purpose":
					json.NewEncoder(w).Encode(registrarMapResponse{1, doc.ReceiptDigest, registrarMap(doc)})
					return
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer s.Close()
			doc = registrarFixture(t, s.URL)
			var c *RegistrarClient
			if kind == "untrusted-tls" {
				var err error
				c, err = NewRegistrarClient(s.URL, doc, RegistrarLimits{time.Second, 1, 1})
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
			} else {
				c = registrarTrusted(t, s, doc, RegistrarLimits{time.Second, 1, 1})
			}
			if err := c.RegistrarEndpoints(context.Background(), nil); err != ErrRegistrarUnavailable {
				t.Fatal("invalid ack/TLS accepted", err)
			}
			expected := int64(1)
			if kind == "untrusted-tls" {
				expected = 0
			}
			if calls.Load() != expected {
				t.Fatal("TLS/retry behavior changed")
			}
		})
	}
}
func FuzzRegistrarCurrentResponseGrammar(f *testing.F) {
	raw, _ := json.Marshal(registrarMapResponse{1, "fixture-digest", &netmap.Map{}})
	f.Add(raw)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var parsed registrarMapResponse
		if !registrarResponseJSON(raw, &parsed) {
			return
		}
		again, _ := json.Marshal(parsed)
		var decoded registrarMapResponse
		if !registrarResponseJSON(again, &decoded) {
			t.Fatal("accepted grammar cannot roundtrip")
		}
	})
}
