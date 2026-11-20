package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

// A Multies device-list credential is deliberately unrelated to the legacy
// operator, account and node credentials. These checks run the real handlers;
// no fake Enroll verifier or pre-approved caller evidence is installed.
func TestMultiesListTokenDoesNotBecomeLegacyAuthority(t *testing.T) {
	st, err := Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	network, err := st.CreateNetwork("separate")
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := NewServer(st, nil, "https://control.test", "operator-only")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("opaque list-only credential, not operator authority"))
	token := base64.RawURLEncoding.EncodeToString(sum[:])
	request := func(method, path string, body any, nodeID string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://control.test"+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		if nodeID != "" {
			r.Header.Set("X-Node-ID", nodeID)
			r.Header.Set("X-Node-Token", token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/admin/api/state", nil},
		{"POST", "/admin/api/networks", map[string]string{"name": "forged"}},
		{"POST", "/admin/api/memberships", map[string]any{"network_id": network.ID, "issuer": "https://multies.test", "subject": "person", "allow": true}},
		{"POST", "/admin/api/nodes/approve", map[string]string{"id": "unknown"}},
		{"GET", "/account/api/devices?network_id=" + network.ID, nil},
	} {
		if w := request(tc.method, tc.path, tc.body, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("list credential reached %s: %d", tc.path, w.Code)
		}
	}
	if len(st.Networks()) != 1 {
		t.Fatal("list credential created a network")
	}
	if _, err = st.IdentityDevices(network.ID, "https://multies.test", "person"); err != ErrUnauthorized {
		t.Fatal("list credential created membership")
	}
	// The public legacy join can still create an unapproved pending record.
	// That is not Multies registration or authority. It must not yield a map,
	// implicit approval provenance, membership or node-token substitution.
	offer := netmap.JoinRequest{Network: network.ID, Name: "pending", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}
	w := request("POST", "/api/join", offer, "")
	if w.Code != 200 {
		t.Fatalf("legacy pending fixture failed: %d", w.Code)
	}
	var pending netmap.JoinResponse
	if json.Unmarshal(w.Body.Bytes(), &pending) != nil || pending.NodeID == "" || pending.Approved {
		t.Fatal("list token caused node approval")
	}
	if _, err = st.Map(pending.NodeID); err != ErrUnauthorized {
		t.Fatal("pending list caller received a network map")
	}
	if _, ok := st.NodeApprovalIdentity(pending.NodeID); ok {
		t.Fatal("list caller supplied verified principal")
	}
	if w = request("GET", "/api/map", nil, pending.NodeID); w.Code != 401 {
		t.Fatalf("list credential replaced saved node credential: %d", w.Code)
	}
	if w = request("POST", "/api/join", offer, pending.NodeID); w.Code != 401 {
		t.Fatalf("list credential refreshed an existing node: %d", w.Code)
	}
	for _, path := range []string{"/api/mnet/register", "/api/mnet/enroll", "/api/mnet/approve"} {
		w = request("POST", path, map[string]string{"credential": token}, "")
		if w.Code != 404 {
			t.Fatalf("unexpected registration alias %s: %d", path, w.Code)
		}
	}
}
