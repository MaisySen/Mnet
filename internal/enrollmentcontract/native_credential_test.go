package enrollmentcontract

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func nativeDocumentFixture(t *testing.T) NativeCredentialDocument {
	t.Helper()
	c, _ := vector(t)
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{73}, 32))
	var err error
	c.Binding.CredentialDigest, err = NativeCredentialSecretDigest(secret)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ReceiptV2ForChallenge(c, "node-fixture", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{74}, 32)), c.Binding.CheckedAtMS+1)
	if err != nil {
		t.Fatal(err)
	}
	return NativeCredentialDocument{Version: 1, Kind: NativeCredentialKind, Secret: secret, FinalizationID: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{75}, 32)), Challenge: c, Receipt: receipt, ReceiptDigest: ReceiptV2Digest(receipt)}
}
func TestNativeCredentialExplicitDocumentRoundtripAndNoAuthority(t *testing.T) {
	doc := nativeDocumentFixture(t)
	raw, _ := json.Marshal(doc)
	got, err := DecodeNativeCredentialDocument(raw)
	if err != nil || got != doc {
		t.Fatal("roundtrip", err)
	}
	// Even a fully coherent altered node + recomputed public digest is DATA.
	changed := doc
	changed.Receipt.NodeID = "caller-fabricated-node"
	changed.ReceiptDigest = ReceiptV2Digest(changed.Receipt)
	if CheckNativeCredentialDocument(changed) != nil {
		t.Fatal("syntax checker pretended to authenticate node origin")
	}
	for _, f := range []string{"%v", "%+v", "%#v"} {
		text := fmt.Sprintf(f, doc)
		if strings.Contains(text, doc.Secret) || strings.Contains(text, doc.Receipt.NodeID) {
			t.Fatal("format disclosed private document")
		}
	}
	// Historical documents retain their exact times, not implicit renewed access.
	if got.Receipt.AccessExpiresAtMS != doc.Challenge.Binding.AccessUntilMS {
		t.Fatal("receipt expiry changed")
	}
}
func TestNativeCredentialEveryReceiptFieldBindsSameChallenge(t *testing.T) {
	original := nativeDocumentFixture(t)
	typ := reflect.TypeOf(original.Receipt)
	for i := 0; i < typ.NumField(); i++ {
		changed := original
		v := reflect.ValueOf(&changed.Receipt).Elem().Field(i)
		switch v.Kind() {
		case reflect.String:
			v.SetString(v.String() + "x")
		case reflect.Int, reflect.Int64:
			v.SetInt(v.Int() + 1)
		}
		// ReceiptID/node/commit are chosen by registrar, so bind to original receipt
		// digest. All other fields must additionally remain exact challenge projection.
		if CheckNativeCredentialDocument(changed) == nil {
			t.Fatal("changed receipt field accepted", typ.Field(i).Name)
		}
		if name := typ.Field(i).Name; name != "ReceiptID" && name != "NodeID" && name != "CommittedAtMS" {
			changed.ReceiptDigest = ReceiptV2Digest(changed.Receipt)
			if CheckNativeCredentialDocument(changed) == nil {
				t.Fatal("rehashed inconsistent receipt accepted", name)
			}
		}
	}
	for _, secret := range []string{"", original.Secret + "=", base64.RawURLEncoding.EncodeToString(make([]byte, 32)), original.Receipt.CredentialDigest} {
		changed := original
		changed.Secret = secret
		if CheckNativeCredentialDocument(changed) == nil {
			t.Fatal("invalid native secret accepted")
		}
	}
}
func TestNativeCredentialStrictRawDocument(t *testing.T) {
	doc := nativeDocumentFixture(t)
	raw, _ := json.Marshal(doc)
	for _, bad := range [][]byte{nil, []byte("null"), append(append([]byte{}, raw...), raw...), []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"version":1.0`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"Version":1`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"version":null`, 1)), []byte(strings.Replace(string(raw), `"version":1,`, "", 1)), []byte(strings.Replace(string(raw), `"node_id":`, `"unknown":0,"node_id":`, 1)), append([]byte{0xff}, raw...), bytes.Repeat([]byte(" "), ProofWireMaxBytes+1)} {
		if got, err := DecodeNativeCredentialDocument(bad); err == nil || got != (NativeCredentialDocument{}) {
			t.Fatal("malformed document accepted or partial secret returned")
		}
	}
	for _, changed := range []NativeCredentialDocument{{}, func() NativeCredentialDocument { d := doc; d.Kind = "legacy"; return d }(), func() NativeCredentialDocument { d := doc; d.Version = 2; return d }()} {
		if CheckNativeCredentialDocument(changed) == nil {
			t.Fatal("wrong schema accepted")
		}
	}
}
func FuzzNativeCredentialDocument(f *testing.F) {
	f.Add([]byte(`{}`))
	if valid, err := os.ReadFile("testdata/native-credential-v1.json"); err == nil {
		f.Add(valid)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		doc, err := DecodeNativeCredentialDocument(raw)
		if err != nil {
			if doc != (NativeCredentialDocument{}) {
				t.Fatal("partial private return")
			}
			return
		}
		if CheckNativeCredentialDocument(doc) != nil {
			t.Fatal("decoder admitted invalid data")
		}
		round, _ := json.Marshal(doc)
		again, err := DecodeNativeCredentialDocument(round)
		if err != nil || again != doc {
			t.Fatal("roundtrip changed syntax DATA")
		}
	})
}

func TestNativeCredentialIndependentPythonReceiptVector(t *testing.T) {
	raw, err := os.ReadFile("testdata/native-credential-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := DecodeNativeCredentialDocument(raw)
	if err != nil {
		t.Fatal("independent native schema fixture refused", err)
	}
	vector, err := os.ReadFile("testdata/receipt-v2.hex")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := hex.DecodeString(strings.TrimSpace(string(vector)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, ReceiptV2Bytes(doc.Receipt)) {
		t.Fatal("receipt bytes differ from independent Python")
	}
	if doc != nativeDocumentFixture(t) {
		t.Fatal("independent full native document differs from Go fixture")
	}
}
