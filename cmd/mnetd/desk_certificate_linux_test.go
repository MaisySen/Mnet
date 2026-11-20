//go:build linux

package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"fintex.au/mnet/internal/desk"
)

// Local disposable material only. Nothing is logged, enrolled or transmitted.
func TestDeskCertificateLoaderMatchesExactRoleAndSingleIdentity(t *testing.T) {
	primary, pk := deskCertificateFixture(t)
	recipient, rk := deskCertificateFixture(t)
	for _, role := range []string{"primary", "recipient"} {
		for _, fault := range []string{"valid", "wrong-pin", "foreign-key", "chain", "invalid-public", "invalid-private", "nonprivate-key"} {
			t.Run(role+"/"+fault, func(t *testing.T) {
				cert, other := primary, recipient
				if role == "recipient" {
					cert, other = recipient, primary
				}
				public := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
				private, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
				if err != nil {
					t.Fatal(err)
				}
				key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
				spec := desk.RunSpec{Role: role, Binding: desk.Binding{Primary: pk, Recipient: rk}}
				switch fault {
				case "wrong-pin":
					if role == "primary" {
						spec.Binding.Primary = rk
					} else {
						spec.Binding.Recipient = pk
					}
				case "foreign-key":
					private, err = x509.MarshalPKCS8PrivateKey(other.PrivateKey)
					if err != nil {
						t.Fatal(err)
					}
					key = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
				case "chain":
					public = append(public, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other.Certificate[0]})...)
				case "invalid-public":
					public = []byte("not a certificate")
				case "invalid-private":
					key = []byte("not a private key")
				}
				dir := t.TempDir()
				spec.Certificate = filepath.Join(dir, "cert.pem")
				spec.PrivateKey = filepath.Join(dir, "key.pem")
				if err = os.WriteFile(spec.Certificate, public, 0600); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(spec.PrivateKey, key, 0600); err != nil {
					t.Fatal(err)
				}
				if fault == "nonprivate-key" {
					if err = os.Chmod(spec.PrivateKey, 0644); err != nil {
						t.Fatal(err)
					}
				}
				got, err := loadDeskCertificate(spec)
				if fault == "valid" {
					expected := pk
					if role == "recipient" {
						expected = rk
					}
					if err != nil || got.Leaf == nil || desk.CertificateKey(got.Leaf) != expected {
						t.Fatal("exact role identity refused", err)
					}
				} else if err == nil || got.PrivateKey != nil || len(got.Certificate) != 0 {
					t.Fatal("invalid local identity returned key material", fault, err)
				}
			})
		}
	}
}
