package control

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestRegistrarStorageExpiredAccessCannotBecomeLegacyRenewal(t *testing.T) {
	s := registrarStorageFixture(t)
	r := registrarFixtureRecord(s)
	r.CommittedAtMS = 1000
	r.AccessExpiresAtMS = 2000
	r.ReceiptDigest = registrarReceiptDigest(*r)
	expiry := time.UnixMilli(r.AccessExpiresAtMS).UTC()
	s.data.Nodes[r.NodeID].KeyExpiry = &expiry
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	s = reopenRegistrarFixture(t, s)
	r = registrarFixtureRecord(s)
	if _, err := s.Map(r.NodeID); err == nil {
		t.Fatal("expired registrar access served map")
	}
	before, _ := json.Marshal(s.data)
	receipt := r.ReceiptDigest
	if err := s.ApproveNode(r.NodeID); err == nil {
		t.Fatal("manual approval renewed expired registrar access")
	}
	after, _ := json.Marshal(s.data)
	if string(before) != string(after) {
		t.Fatal("rejected legacy approval changed receipt/expiry")
	}
	// Local expiry can retire an already elapsed access window without setting a
	// later expiry, silently extending it or losing the immutable original receipt.
	if err := s.ExpireNode(r.NodeID); err != nil {
		t.Fatal(err)
	}
	if r.Phase != "revoked" || r.ReceiptDigest != receipt || s.data.Nodes[r.NodeID].KeyExpiry.UnixMilli() > 2000 {
		t.Fatal("expire extended/changed historical grant")
	}
	s = reopenRegistrarFixture(t, s)
	r = registrarFixtureRecord(s)
	if r.Phase != "revoked" || r.ReceiptDigest != receipt {
		t.Fatal("restart lost elapsed revocation history")
	}
	if err := s.DeleteNode(r.NodeID); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded storeData
	if err := json.Unmarshal(saved, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Registrar == nil || len(decoded.Registrar.Records) != 1 || len(decoded.Nodes) != 0 {
		t.Fatal("delete pruned standing claim")
	}
}
