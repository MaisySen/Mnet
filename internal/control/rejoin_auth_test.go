package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestRejoinRequiresSavedNodeCredential(t *testing.T) {
	st, err := Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateNetwork("home"); err != nil {
		t.Fatal(err)
	}
	offer := netmap.JoinRequest{Network: "home", Name: "owner", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}
	first, err := st.Join(offer)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ApproveNode(first.NodeID); err != nil {
		t.Fatal(err)
	}
	s, _, err := NewServer(st, nil, "https://control.test", "pw")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, id, token string
		want            int
	}{
		{"no credential", "", "", 401},
		{"guessed credential", first.NodeID, "wrong", 401},
		{"wrong identity", "wrong-id", first.NodeToken, 401},
		{"owner", first.NodeID, first.NodeToken, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Even correct credentials in JSON are ignored; only the
			// request's explicit authentication headers bind this join.
			body, _ := json.Marshal(map[string]any{"network": offer.Network, "name": offer.Name, "pubkey": offer.PubKey, "node_id": first.NodeID, "node_token": first.NodeToken})
			r := httptest.NewRequest("POST", "/api/join", bytes.NewReader(body))
			r.Header.Set("X-Node-ID", tc.id)
			r.Header.Set("X-Node-Token", tc.token)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, r)
			if rec.Code != tc.want {
				t.Fatalf("join code %d want %d", rec.Code, tc.want)
			}
			if rec.Code != 200 && bytes.Contains(rec.Body.Bytes(), []byte(first.NodeToken)) {
				t.Fatal("node credential disclosed")
			}
		})
	}
}

func TestExpiredRejoinNeedsFreshEnrollmentAuthorization(t *testing.T) {
	st, err := Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	network, err := st.CreateNetwork("home")
	if err != nil {
		t.Fatal(err)
	}
	key, err := st.CreateAuthKey(network.ID, "fixture", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	req := netmap.JoinRequest{Network: "home", Name: "owner", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), AuthKey: key.Key}
	first, err := st.Join(req)
	if err != nil {
		t.Fatal(err)
	}
	req.NodeID = first.NodeID
	req.NodeToken = first.NodeToken
	if err = st.ExpireNode(first.NodeID); err != nil {
		t.Fatal(err)
	}
	// The valid enrollment key plus saved node credential permits renewal.
	renewed, err := st.Join(req)
	if err != nil {
		t.Fatal(err)
	}
	if !renewed.Approved || renewed.NodeID != first.NodeID {
		t.Fatal("fresh enrollment authorization failed")
	}
	// A different credential cannot steal identity even with an auth key.
	before := key.Uses
	req.NodeToken = "wrong"
	if _, err = st.Join(req); err != ErrUnauthorized {
		t.Fatalf("key-only takeover: %v", err)
	}
	if key.Uses != before {
		t.Fatal("unauthorized request consumed enrollment key")
	}
}
