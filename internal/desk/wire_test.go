package desk

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"math/big"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func PairTestCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "disposable desk fixture"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: private, Leaf: leaf}, CertificateKey(leaf)
}
func pairTransportFixture(t *testing.T) (*Compiled, Binding, tls.Certificate, tls.Certificate) {
	t.Helper()
	primary, pk := PairTestCertificate(t)
	recipient, rk := PairTestCertificate(t)
	layout, err := Compile(Layout{Version: 1, Displays: []Display{{ID: "primary", Device: pk, Width: 100, Height: 100, PixelWidth: 100, PixelHeight: 100}, {ID: "recipient", Device: rk, X: 100, Width: 100, Height: 100, PixelWidth: 200, PixelHeight: 200}}})
	if err != nil {
		t.Fatal(err)
	}
	b := Binding{Run: strings.Repeat("a", 64), Primary: pk, Recipient: rk, Layout: layout.Fingerprint(), Keyboard: strings.Repeat("b", 64), Display: "recipient"}
	return layout, b, primary, recipient
}
func tlsFixturePair(t *testing.T, primary, recipient tls.Certificate) (*tls.Conn, *tls.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	a, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	b := <-accepted
	// Verification is exact application pinning on both ends, not CA/DNS trust.
	return tls.Client(a, &tls.Config{Certificates: []tls.Certificate{primary}, InsecureSkipVerify: true, MinVersion: tls.VersionTLS13, NextProtos: []string{wireVersion}}), tls.Server(b, &tls.Config{Certificates: []tls.Certificate{recipient}, ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS13, NextProtos: []string{wireVersion}})
}
func TestWirePairExactConsentInputAndJoinedStop(t *testing.T) {
	layout, binding, primary, recipient := pairTransportFixture(t)
	clientConn, serverConn := tlsFixturePair(t, primary, recipient)
	backend := &ownedFixture{}
	approval, err := NewApproval(binding, layout, func() bool { return true }, func(func() bool) (NativeInput, error) { return backend, nil })
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- approval.ServePair(serverConn) }()
	client, err := ConnectPair(clientConn, binding, layout, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Apply(Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}); err != nil {
		t.Fatal(err)
	}
	if err = client.Release(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if backend.released != 1 || len(backend.events) != 1 {
		t.Fatal("stop not joined to native cleanup")
	}
	if err = client.Apply(Input{Sequence: 2, Kind: "key", Action: "down", Code: 39}); err == nil {
		t.Fatal("closed pair resumed")
	}
	// The same Approval cannot be consumed by a second TLS connection.
	c, s := tlsFixturePair(t, primary, recipient)
	go func() { done <- approval.ServePair(s) }()
	if _, err = ConnectPair(c, binding, layout, func() bool { return true }); err == nil {
		t.Fatal("consumed approval reused")
	}
	if err = <-done; err == nil {
		t.Fatal("reused approval accepted")
	}
}
func TestWirePairRefusesUnapprovedRunAndForeignKey(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "run", true: "key"}[foreign], func(t *testing.T) {
			layout, binding, primary, recipient := pairTransportFixture(t)
			approved := binding
			if foreign {
				primary, _ = PairTestCertificate(t)
			} else {
				approved.Run = strings.Repeat("c", 64)
			}
			var created atomic.Int32
			approval, _ := NewApproval(approved, layout, func() bool { return true }, func(func() bool) (NativeInput, error) { created.Add(1); return &ownedFixture{}, nil })
			c, s := tlsFixturePair(t, primary, recipient)
			done := make(chan error, 1)
			go func() { done <- approval.ServePair(s) }()
			if client, err := ConnectPair(c, binding, layout, func() bool { return true }); err == nil {
				client.Release()
				t.Fatal("foreign pair admitted")
			}
			if err := <-done; err == nil || created.Load() != 0 {
				t.Fatal("unapproved pair created native backend")
			}
		})
	}
}
func TestWirePairIdleAuthorityLossReleases(t *testing.T) {
	layout, binding, primary, recipient := pairTransportFixture(t)
	var live atomic.Bool
	live.Store(true)
	backend := &ownedFixture{}
	approval, _ := NewApproval(binding, layout, live.Load, func(func() bool) (NativeInput, error) { return backend, nil })
	c, s := tlsFixturePair(t, primary, recipient)
	done := make(chan error, 1)
	go func() { done <- approval.ServePair(s) }()
	client, err := ConnectPair(c, binding, layout, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Apply(Input{Sequence: 1, Kind: "button", Action: "down", Code: 1}); err != nil {
		t.Fatal(err)
	}
	live.Store(false)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle authority loss not terminal")
	}
	if backend.released != 1 {
		t.Fatal("idle pair loss did not release")
	}
	if err = client.Release(); err == nil {
		t.Fatal("unconfirmed release reported clean")
	}
}
func TestWireStrictBounds(t *testing.T) {
	for _, data := range []string{`{"kind":"pulse","kind":"input"}`, `{"kind":"pulse","text":"forbidden"}`, `{"kind":"pulse"} {}`, `null`, strings.Repeat("[", 10) + strings.Repeat("]", 10)} {
		var frame bytes.Buffer
		binary.Write(&frame, binary.BigEndian, uint32(len(data)))
		frame.WriteString(data)
		_, err := readWire(&frame)
		if err == nil {
			t.Fatal("invalid wire JSON accepted")
		}
	}
	var frame bytes.Buffer
	binary.Write(&frame, binary.BigEndian, uint32(4097))
	if _, err := readWire(&frame); err == nil {
		t.Fatal("oversized wire frame accepted")
	}
}

func TestWirePairMissingHeartbeatRetiresHeldInput(t *testing.T) {
	layout, binding, primary, recipient := pairTransportFixture(t)
	backend := &ownedFixture{}
	approval, _ := NewApproval(binding, layout, func() bool { return true }, func(func() bool) (NativeInput, error) { return backend, nil })
	c, s := tlsFixturePair(t, primary, recipient)
	defer c.NetConn().Close()
	done := make(chan error, 1)
	go func() { done <- approval.ServePair(s) }()
	if err := authenticate(c, binding.Recipient); err != nil {
		t.Fatal(err)
	}
	if err := writeWire(c, wireMessage{Kind: "hello", Binding: &binding}); err != nil {
		t.Fatal(err)
	}
	if m, err := readWire(c); err != nil || m.Kind != "ready" {
		t.Fatal("not ready", err)
	}
	if err := writeWire(c, wireMessage{Kind: "input", Input: &Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}}); err != nil {
		t.Fatal(err)
	}
	if m, err := readWire(c); err != nil || !m.OK {
		t.Fatal("input unacknowledged", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("missing heartbeat ended cleanly")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lost heartbeat did not retire")
	}
	if backend.released != 1 {
		t.Fatal("lost heartbeat left held input")
	}
}
func TestWirePairReleaseFailureNeverAcknowledgesSuccess(t *testing.T) {
	layout, binding, primary, recipient := pairTransportFixture(t)
	backend := &ownedFixture{releaseErr: errors.New("native release failed")}
	approval, _ := NewApproval(binding, layout, func() bool { return true }, func(func() bool) (NativeInput, error) { return backend, nil })
	c, s := tlsFixturePair(t, primary, recipient)
	done := make(chan error, 1)
	go func() { done <- approval.ServePair(s) }()
	client, err := ConnectPair(c, binding, layout, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Apply(Input{Sequence: 1, Kind: "button", Action: "down", Code: 1}); err != nil {
		t.Fatal(err)
	}
	if err = client.Release(); err == nil {
		t.Fatal("failed cleanup acknowledged")
	}
	if err = client.Release(); err == nil {
		t.Fatal("cleanup failure forgotten")
	}
	if err = <-done; err == nil || backend.released != 1 {
		t.Fatal("native cleanup failure lost")
	}
}
