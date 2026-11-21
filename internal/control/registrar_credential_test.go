package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

func registrarCredentialFixture(t *testing.T) (*Store, registrarCredentialReference, string) {
	t.Helper()
	s := registrarConsumptionFixture(t)
	r := registrarFixtureRecord(s)
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32))
	sum := sha256.Sum256([]byte(secret))
	r.CredentialDigest = base64.RawURLEncoding.EncodeToString(sum[:])
	r.Consumption.Challenge.Binding.CredentialDigest = r.CredentialDigest
	r.Consumption.TranscriptDigest = registrarTranscriptDigest(r.Consumption.Challenge)
	r.ReceiptDigest = registrarReceiptDigest(*r)
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	s = reopenRegistrarFixture(t, s)
	r = registrarFixtureRecord(s)
	ref := registrarCredentialReference{NodeID: r.NodeID, NetworkID: r.NetworkID, DevicePublicKey: r.DevicePublicKey, Scope: r.Scope, ReceiptID: r.ReceiptID, ReceiptDigest: r.ReceiptDigest, DescriptorDigest: r.DescriptorDigest, TranscriptDigest: r.Consumption.TranscriptDigest}
	return s, ref, secret
}
func TestRegistrarCredentialActualReopenedRecordSecretScopeAndLegacySeparation(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	before, _ := os.ReadFile(s.path)
	version := s.Version()
	got, err := s.authorizeRegistrarCredential(ref, secret)
	if err != nil || got.ID != ref.NodeID {
		t.Fatal("own strict record credential refused", err)
	}
	got.Name = "mutated copy"
	if s.data.Nodes[ref.NodeID].Name == got.Name {
		t.Fatal("node snapshot aliased authority")
	}
	if _, err := s.AuthorizeNode(ref.NodeID, secret); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("registrar secret accepted as legacy token", err)
	}
	for _, wrong := range []string{"", secret + "=", registrarFixtureRecord(s).CredentialDigest, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{43}, 32)), "not-canonical"} {
		if _, err := s.authorizeRegistrarCredential(ref, wrong); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("wrong secret accepted", err)
		}
	}
	after, _ := os.ReadFile(s.path)
	if !bytes.Equal(before, after) || s.Version() != version {
		t.Fatal("credential read mutated Store")
	}
}
func TestRegistrarCredentialEveryQualifiedScopeFieldBound(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	for _, target := range []string{"reference", "scope"} {
		value := reflect.ValueOf(ref)
		if target == "scope" {
			value = reflect.ValueOf(ref.Scope)
		}
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).Kind() != reflect.String {
				continue
			}
			changed := ref
			field := reflect.ValueOf(&changed).Elem()
			if target == "scope" {
				field = field.FieldByName("Scope")
			}
			field = field.Field(i)
			field.SetString(field.String() + "x")
			if _, err := s.authorizeRegistrarCredential(changed, secret); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("qualified field ignored", target, value.Type().Field(i).Name, err)
			}
		}
	}
}
func TestRegistrarCredentialCurrentAuthorityAndCorruptionRefuse(t *testing.T) {
	for _, kind := range []string{"revoked", "expired", "missing-node", "missing-network", "unapproved", "mixed-token", "routes", "exit", "account-user", "wrong-node-key", "wrong-expiry", "consumed-index", "receipt", "transcript", "legacy-v1", "closed", "frozen", "released-owner"} {
		t.Run(kind, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			r := registrarFixtureRecord(s)
			n := s.data.Nodes[ref.NodeID]
			switch kind {
			case "revoked":
				if err := s.ExpireNode(ref.NodeID); err != nil {
					t.Fatal(err)
				}
			case "expired":
				r.AccessExpiresAtMS = time.Now().Add(-time.Millisecond).UnixMilli()
				end := time.UnixMilli(r.AccessExpiresAtMS)
				n.KeyExpiry = &end
			case "missing-node":
				delete(s.data.Nodes, ref.NodeID)
			case "missing-network":
				delete(s.data.Networks, r.NetworkID)
			case "unapproved":
				n.Approved = false
			case "mixed-token":
				s.data.NodeTokens[ref.NodeID] = secret
			case "routes":
				n.Routes = []string{"0.0.0.0/0"}
			case "exit":
				n.ExitNode = true
			case "account-user":
				n.User = "unverified-person"
			case "wrong-node-key":
				n.PubKey = "wrong"
			case "wrong-expiry":
				end := n.KeyExpiry.Add(time.Second)
				n.KeyExpiry = &end
			case "consumed-index":
				delete(s.data.Registrar.ConsumedProofs, registrarProofKey(r.Consumption.Challenge))
			case "receipt":
				r.ReceiptDigest = registrarContractDigest(12)
			case "transcript":
				r.Consumption.TranscriptDigest = registrarContractDigest(13)
			case "legacy-v1":
				s.data.Registrar.Version = 1
			case "closed":
				s.Close()
			case "frozen":
				s.mu.Lock()
				s.freezeLocked(ErrStoreUncertain)
				s.mu.Unlock()
			case "released-owner":
				s.lease.owner.Close()
			}
			before, _ := json.Marshal(s.data)
			if _, err := s.authorizeRegistrarCredential(ref, secret); err == nil {
				t.Fatal("inactive/corrupt owner authorized")
			}
			after, _ := json.Marshal(s.data)
			if !bytes.Equal(before, after) {
				t.Fatal("failed verifier repaired/mutated state")
			}
		})
	}
}
