package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestRegistrarStorageLegacyScopeAndHistoryPreservation(t *testing.T) {
	s := registrarStorageFixture(t)
	r := *registrarFixtureRecord(s)
	other, err := s.CreateNetwork("other")
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []struct{ network, key string }{{r.NetworkID, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{10}, 32))}, {other.ID, r.DevicePublicKey}} {
		key, err := s.CreateAuthKey(scope.network, "independent legacy", true, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.Join(netmap.JoinRequest{Network: scope.network, Name: "legacy", PubKey: scope.key, AuthKey: key.Key})
		if err != nil || !result.Approved {
			t.Fatal("registrar claim widened into unrelated legacy scope", err)
		}
		if err := s.DeleteNode(result.NodeID); err != nil {
			t.Fatal(err)
		}
		result, err = s.Join(netmap.JoinRequest{Network: scope.network, Name: "legacy", PubKey: scope.key, AuthKey: key.Key})
		if err != nil || !result.Approved {
			t.Fatal("unclaimed reusable-key policy changed", err)
		}
	}
	s = reopenRegistrarFixture(t, s)
	saved := s.registrarClaimLocked(r.NetworkID, r.DevicePublicKey)
	if saved == nil || *saved != r {
		t.Fatal("unrelated legacy mutation changed immutable registrar record")
	}
}

func TestRegistrarStorageRejectsDuplicateAndCaseAliasedFields(t *testing.T) {
	s := registrarStorageFixture(t)
	valid, err := json.Marshal(s.data)
	if err != nil {
		t.Fatal(err)
	}
	path := s.path
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cases := [][]byte{
		bytes.Replace(valid, []byte(`"receipt_encoding_version":1`), []byte(`"receipt_encoding_version":1,"receipt_encoding_version":1`), 1),
		bytes.Replace(valid, []byte(`"phase":"active"`), []byte(`"Phase":"active"`), 1),
		bytes.Replace(valid, []byte(`"service_id":"fixture-service"`), []byte(`"Service_ID":"fixture-service"`), 1),
		bytes.Replace(valid, []byte(`"registrar_state":`), []byte(`"Registrar_State":`), 1),
		[]byte(`{"registrar_state":null}`),
		[]byte(`{"Registrar_State":null}`),
		[]byte(`{"registrar_state":{"version":1,"records":{}},"registrar_state":{"version":1,"records":{}}}`),
	}
	for i, data := range cases {
		if bytes.Equal(data, valid) {
			t.Fatal("fixture replacement did not modify source", i)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		opened, err := Open(path)
		if opened != nil {
			opened.Close()
		}
		if err == nil || opened != nil {
			t.Errorf("aliased/duplicate authority field accepted case %d", i)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, data) {
			t.Fatal("rejected source changed", i, err)
		}
	}
}

func TestRegistrarStorageEmptyNamespaceAndLegacyOmissionStayDistinct(t *testing.T) {
	for _, original := range []string{`{}`, `{"registrar_state":{"version":1,"records":{}}}`} {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := openTestStore(t, path)
		if err != nil {
			t.Fatal(err)
		}
		wanted := original != "{}"
		if (s.data.Registrar != nil) != wanted {
			t.Fatal("namespace presence changed on load")
		}
		if _, err := s.CreateNetwork("legacy"); err != nil {
			t.Fatal(err)
		}
		s = reopenRegistrarFixture(t, s)
		if (s.data.Registrar != nil) != wanted {
			t.Fatal("unrelated save reset/invented authority namespace")
		}
	}
}
