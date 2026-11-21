package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestJoinKeyEncodingCannotCreateAliasOfExistingIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	network, err := st.CreateNetwork("fixture")
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	original, err := st.Join(netmap.JoinRequest{Network: network.ID, Name: "owner", PubKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ApproveNode(original.NodeID); err != nil {
		t.Fatal(err)
	}
	app, _, err := NewServer(st, nil, "http://control.invalid", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{key[:12] + "\n" + key[12:], key[:12] + "\r\n" + key[12:], key[:42] + "B="} {
		decoded, err := base64.StdEncoding.DecodeString(alias)
		if err != nil || !bytes.Equal(decoded, make([]byte, 32)) {
			t.Fatal("fixture is not an alias of the original key", err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		version := st.Version()
		cursor := st.data.NextIP[network.ID]
		body, _ := json.Marshal(netmap.JoinRequest{Network: network.ID, Name: "alias", PubKey: alias})
		req := httptest.NewRequest(http.MethodPost, "http://control.invalid/api/join", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Error("noncanonical identity alias accepted", response.Code)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) || st.Version() != version || len(st.data.Nodes) != 1 || st.data.NextIP[network.ID] != cursor {
			t.Error("alias allocated saved identity or changed map version")
		}
		if strings.Contains(response.Body.String(), original.NodeToken) {
			t.Error("alias response exposed owner credential")
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := openTestStore(t, path)
		if err != nil {
			t.Fatal(err)
		}
		st = reopened
		app.store = reopened
		if len(reopened.data.Nodes) != 1 {
			t.Error("alias survived reopen")
		}
	}
}

func TestJoinKeyEncodingCanonicalContract(t *testing.T) {
	for _, first := range []byte{0, 1, 254, 255} {
		raw := bytes.Repeat([]byte{first}, 32)
		key := base64.StdEncoding.EncodeToString(raw)
		if !validPubKey(key) {
			t.Fatal("canonical current key rejected")
		}
	}
	for _, key := range []string{"", base64.StdEncoding.EncodeToString(make([]byte, 31)), base64.StdEncoding.EncodeToString(make([]byte, 33)), "not-a-key"} {
		if validPubKey(key) {
			t.Fatal("invalid key accepted")
		}
	}
}
