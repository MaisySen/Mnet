package desk

import (
	"crypto/tls"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// Disposable TCP/TLS only. A valid key is insufficient without the exact
// protocol, version and single-certificate shape. No native owner is created.
func TestWireAdmissionRefusesVersionProtocolAndCertificateShape(t *testing.T) {
	for _, fault := range []string{"tls12", "wrong-alpn", "missing-alpn", "no-client-cert", "client-chain", "server-chain", "foreign-server"} {
		t.Run(fault, func(t *testing.T) {
			layout, binding, primary, recipient := pairTransportFixture(t)
			if fault == "client-chain" {
				primary.Certificate = append(primary.Certificate, primary.Certificate[0])
			}
			if fault == "server-chain" {
				recipient.Certificate = append(recipient.Certificate, recipient.Certificate[0])
			}
			if fault == "foreign-server" {
				recipient, _ = PairTestCertificate(t)
			}
			clientCfg := &tls.Config{Certificates: []tls.Certificate{primary}, InsecureSkipVerify: true, MinVersion: tls.VersionTLS13, NextProtos: []string{wireVersion}}
			serverCfg := &tls.Config{Certificates: []tls.Certificate{recipient}, ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS13, NextProtos: []string{wireVersion}}
			switch fault {
			case "tls12":
				clientCfg.MinVersion = tls.VersionTLS12
				clientCfg.MaxVersion = tls.VersionTLS12
				serverCfg.MinVersion = tls.VersionTLS12
				serverCfg.MaxVersion = tls.VersionTLS12
			case "wrong-alpn":
				clientCfg.NextProtos = []string{"not-desk"}
				serverCfg.NextProtos = []string{"not-desk"}
			case "missing-alpn":
				clientCfg.NextProtos = nil
				serverCfg.NextProtos = nil
			case "no-client-cert":
				clientCfg.Certificates = nil
			}
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
			raw, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			var peer net.Conn
			select {
			case peer = <-accepted:
			case <-time.After(time.Second):
				t.Fatal("fixture accept did not join")
			}
			defer peer.Close()
			var created atomic.Int32
			approval, err := NewApproval(binding, layout, func() bool { return true }, func(func() bool) (NativeInput, error) { created.Add(1); return &ownedFixture{}, nil })
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- approval.ServePair(tls.Server(peer, serverCfg)) }()
			client, err := ConnectPair(tls.Client(raw, clientCfg), binding, layout, func() bool { return true })
			if err == nil || client != nil {
				if client != nil {
					client.Release()
				}
				t.Fatal("invalid transport admitted", fault)
			}
			select {
			case err = <-done:
				if err == nil {
					t.Fatal("server reported successful invalid transport")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("invalid transport left server alive")
			}
			if created.Load() != 0 {
				t.Fatal("invalid TLS created native backend", created.Load())
			}
		})
	}
}
