package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRegistrarReceiptV1IndependentPythonBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/registrar-receipt-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		FieldCount int             `json:"field_count"`
		ByteLength int             `json:"byte_length"`
		SHA256     string          `json:"sha256_hex"`
		Record     registrarRecord `json:"record"`
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	hexRaw, err := os.ReadFile("testdata/registrar-receipt-v1.hex")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := hex.DecodeString(strings.TrimSpace(string(hexRaw)))
	if err != nil {
		t.Fatal(err)
	}
	actual := registrarReceiptBytes(vector.Record)
	if vector.FieldCount != 19 || len(actual) != vector.ByteLength || !bytes.Equal(actual, expected) {
		t.Fatal("Go bytes differ from independently packed Python receipt")
	}
	digest := sha256.Sum256(actual)
	if hex.EncodeToString(digest[:]) != vector.SHA256 || registrarReceiptDigest(vector.Record) != vector.Record.ReceiptDigest {
		t.Fatal("receipt digest differs from independent vector")
	}
	if !bytes.Contains(actual, []byte("<>&-é-\u2028-\u2029")) {
		t.Fatal("receipt lost exact HTML/non-ASCII/Unicode separator bytes")
	}
	// Revocation changes current access, never rewrites immutable receipt history.
	changed := vector.Record
	changed.Phase = "active"
	if !bytes.Equal(registrarReceiptBytes(changed), actual) {
		t.Fatal("mutable phase changed immutable receipt")
	}
	for _, mutate := range []func(*registrarRecord){
		func(r *registrarRecord) { r.Scope.Issuer += "/other" }, func(r *registrarRecord) { r.Scope.RegistrarOrigin += "x" },
		func(r *registrarRecord) { r.Scope.Audience += "x" }, func(r *registrarRecord) { r.Scope.ServiceID += "x" },
		func(r *registrarRecord) { r.Scope.WorkspaceID += "x" }, func(r *registrarRecord) { r.Scope.MappingRevision += "x" },
		func(r *registrarRecord) { r.Scope.HandoffID = registrarContractDigest(30) }, func(r *registrarRecord) { r.Scope.AttemptID = registrarContractDigest(30) },
		func(r *registrarRecord) { r.Scope.ReserveID = registrarContractDigest(30) }, func(r *registrarRecord) { r.DescriptorDigest = registrarContractDigest(30) },
		func(r *registrarRecord) { r.ReceiptID = registrarContractDigest(30) }, func(r *registrarRecord) { r.NodeID += "x" },
		func(r *registrarRecord) { r.NetworkID += "x" }, func(r *registrarRecord) { r.DevicePublicKey += "x" },
		func(r *registrarRecord) { r.CredentialDigest = registrarContractDigest(30) }, func(r *registrarRecord) { r.CommittedAtMS++ },
		func(r *registrarRecord) { r.AccessExpiresAtMS++ },
	} {
		changed := vector.Record
		mutate(&changed)
		if bytes.Equal(registrarReceiptBytes(changed), actual) || registrarReceiptDigest(changed) == vector.Record.ReceiptDigest {
			t.Fatal("immutable receipt field was not bound")
		}
	}
	changed = vector.Record
	changed.Scope.Audience = "ab"
	changed.Scope.ServiceID = "c"
	one := registrarReceiptBytes(changed)
	changed.Scope.Audience = "a"
	changed.Scope.ServiceID = "bc"
	if bytes.Equal(one, registrarReceiptBytes(changed)) {
		t.Fatal("field boundary shift collided")
	}
}

func TestRegistrarReceiptV2IndependentPythonBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/registrar-receipt-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		FieldCount int             `json:"field_count"`
		ByteLength int             `json:"byte_length"`
		SHA256     string          `json:"sha256_hex"`
		Record     registrarRecord `json:"record"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("testdata/registrar-receipt-v2.hex")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	actual := registrarReceiptBytes(v.Record)
	hash := sha256.Sum256(actual)
	if v.FieldCount != 21 || len(actual) != v.ByteLength || !bytes.Equal(actual, expected) || hex.EncodeToString(hash[:]) != v.SHA256 || registrarReceiptDigest(v.Record) != v.Record.ReceiptDigest {
		t.Fatal("v2 Go differs from independent Python bytes")
	}
	if v.Record.Consumption.TranscriptDigest != registrarTranscriptDigest(v.Record.Consumption.Challenge) {
		t.Fatal("consumed transcript digest differs from actual codec")
	}
	changed := v.Record
	copy := *changed.Consumption
	changed.Consumption = &copy
	changed.Consumption.TranscriptDigest = registrarContractDigest(31)
	if bytes.Equal(actual, registrarReceiptBytes(changed)) {
		t.Fatal("v2 receipt omitted consumed proof binding")
	}
	changed = v.Record
	changed.ReceiptEncodingVersion = 1
	changed.Consumption = nil
	if bytes.Equal(actual, registrarReceiptBytes(changed)) {
		t.Fatal("receipt version domain separation failed")
	}
}
