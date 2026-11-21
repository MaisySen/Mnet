//go:build mnet_enrollment_development

// Disposable loopback DEVELOPMENT service only. Not a production daemon,
// installer, owner-authority verifier, native OS integration or public server.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fintex.au/mnet/internal/control"
	"fintex.au/mnet/internal/privatefs"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "disposable registrar stopped:", e)
		os.Exit(1)
	}
}
func run() error {
	parent := flag.String("parent", os.TempDir(), "parent for NEW disposable private temporary root")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("no positional arguments accepted")
	}
	d, e := control.NewDisposableRegistrar(*parent)
	if e != nil {
		return e
	}
	defer d.Close()
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return e
	}
	defer listener.Close()
	origin := "https://" + listener.Addr().String()
	cert, certPEM, e := localCertificate()
	if e != nil {
		return e
	}
	certPath := filepath.Join(d.Root(), "local-tls-certificate.pem")
	if e = privatefs.WriteFile(certPath, certPEM); e != nil {
		return e
	}
	encoder := json.NewEncoder(os.Stdout)
	if e = encoder.Encode(map[string]any{"stage": "awaiting-disposable-configuration", "root": d.Root(), "networkId": d.NetworkID(), "origin": origin, "certificatePath": certPath, "hostedReady": false, "durableQuota": false}); e != nil {
		return e
	}
	// One bounded configuration followed by EOF. Service bearer bytes never go to
	// stdout/errors. Exact owning parsers validate both RawMessage values.
	raw, e := io.ReadAll(io.LimitReader(os.Stdin, 131073))
	if e != nil || len(raw) > 131072 {
		return fmt.Errorf("bounded configuration required")
	}
	input, e := control.DecodeDisposableRegistrarConfiguration(raw)
	if e != nil {
		return e
	}
	if e = d.Configure(input.Sources, input.Workflow, origin); e != nil {
		return fmt.Errorf("disposable configuration refused")
	}
	handler, e := d.Handler()
	if e != nil {
		return e
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16384, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}, Certificates: []tls.Certificate{cert}}}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-signals:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			server.Shutdown(ctx)
		case <-done:
		}
	}()
	if e = encoder.Encode(map[string]any{"stage": "disposable-service-ready", "origin": origin, "hostedReady": false, "durableQuota": false}); e != nil {
		return e
	}
	e = server.Serve(tls.NewListener(listener, server.TLSConfig))
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
func localCertificate() (tls.Certificate, []byte, error) {
	var empty tls.Certificate
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return empty, nil, e
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		return empty, nil, e
	}
	template := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "disposable registrar loopback"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, e := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if e != nil {
		return empty, nil, e
	}
	keyDER, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		return empty, nil, e
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert, e := tls.X509KeyPair(certPEM, keyPEM)
	clear(keyDER)
	clear(keyPEM)
	return cert, certPEM, e
}
