package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func nativeCandidateFixture(t *testing.T) (*Daemon, []byte) {
	t.Helper()
	d, _, _ := enrollmentContextFixture(t)
	var v struct {
		Challenge contract.Challenge `json:"challenge"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "enrollmentcontract", "testdata", "transcript-v4.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	c := v.Challenge
	b := &c.Binding
	b.PrincipalAuthority = "https://principal.test"
	b.PrincipalNamespace = contract.PrincipalNamespace
	b.PrincipalID = "local-user"
	b.IssuerLabel = "https://issuer.test/path"
	b.RegistrarOrigin = "https://registrar.test"
	b.Audience = "fixture"
	b.ServiceID = "fixture-service"
	b.WorkspaceID = "workspace"
	b.NetworkID = "network"
	b.MappingRevision = "r1"
	key, _ := base64.StdEncoding.Strict().DecodeString(d.State().PublicKey)
	b.DevicePublicKey = base64.RawURLEncoding.EncodeToString(key)
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{91}, 32))
	b.CredentialDigest, err = contract.NativeCredentialSecretDigest(secret)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := contract.ReceiptV2ForChallenge(c, "fixture-node", c.Nonce, c.Binding.CheckedAtMS+1)
	if err != nil {
		t.Fatal(err)
	}
	doc := contract.NativeCredentialDocument{Version: 1, Kind: contract.NativeCredentialKind, Secret: secret, FinalizationID: c.ID, Challenge: c, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)}
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return d, raw
}
func TestNativeCredentialCandidateActualOwnerPolicyKeyNoPublication(t *testing.T) {
	d, raw := nativeCandidateFixture(t)
	path := d.stateOwner.path
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := d.stateOwner.lease.Parent()
	entries, _ := os.ReadDir(dir)
	d.mu.Lock()
	candidate, err := d.prepareCredentialCandidateLocked(raw)
	d.mu.Unlock()
	if err != nil {
		t.Fatal("candidate DATA", err)
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, candidate), candidate.document.Secret) {
			t.Fatal("candidate formatting disclosed secret")
		}
	}
	other, _ := nativeCandidateFixture(t)
	other.mu.Lock()
	err = other.checkCredentialCandidateLocked(candidate)
	other.mu.Unlock()
	if err == nil {
		t.Fatal("foreign actual owner accepted candidate")
	}
	after, _ := os.ReadFile(path)
	afterEntries, _ := os.ReadDir(dir)
	if !bytes.Equal(before, after) || len(entries) != len(afterEntries) {
		t.Fatal("candidate published credential/pending state")
	}
	// Historical document is deliberately from a past fixture clock. Candidate
	// acceptance is consistency only, never a live-access/receipt-origin claim.
	d.mu.Lock()
	err = d.checkCredentialCandidateLocked(candidate)
	d.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "enrollment-policy-v2.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	err = d.checkCredentialCandidateLocked(candidate)
	d.mu.Unlock()
	if err == nil {
		t.Fatal("changed installed policy accepted candidate")
	}
}
func TestNativeCredentialCandidateRefusesChangedKeyPrincipalAndLegacyIdentity(t *testing.T) {
	for _, kind := range []string{"principal", "key", "mixed-authkey", "routes", "closed", "mutated-document"} {
		t.Run(kind, func(t *testing.T) {
			d, raw := nativeCandidateFixture(t)
			switch kind {
			case "principal", "key":
				var doc contract.NativeCredentialDocument
				if err := json.Unmarshal(raw, &doc); err != nil {
					t.Fatal(err)
				}
				if kind == "principal" {
					doc.Challenge.Binding.PrincipalID = "other-user"
				} else {
					doc.Challenge.Binding.DevicePublicKey = doc.Challenge.Nonce
				}
				receipt, err := contract.ReceiptV2ForChallenge(doc.Challenge, doc.Receipt.NodeID, doc.Receipt.ReceiptID, doc.Receipt.CommittedAtMS)
				if err != nil {
					t.Fatal(err)
				}
				doc.Receipt = receipt
				doc.ReceiptDigest = contract.ReceiptV2Digest(receipt)
				raw, _ = json.Marshal(doc)
			case "mixed-authkey":
				d.mu.Lock()
				d.state.AuthKey = "test-only-legacy-key"
				if err := d.stateOwner.save(d.state); err != nil {
					t.Fatal(err)
				}
				d.mu.Unlock()
			case "routes":
				if err := d.SetRouting(false, []string{"10.0.0.0/24"}, ""); err != nil {
					t.Fatal(err)
				}
			case "closed":
				d.Close()
			}
			d.mu.Lock()
			candidate, err := d.prepareCredentialCandidateLocked(raw)
			if kind == "mutated-document" && err == nil {
				candidate.document.Secret = candidate.document.FinalizationID
				err = d.checkCredentialCandidateLocked(candidate)
			}
			d.mu.Unlock()
			if err == nil {
				t.Fatal("changed/ineligible candidate accepted", kind)
			}
		})
	}
}
