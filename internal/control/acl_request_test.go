package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestACLRequestRejectsAmbiguityWithoutClearingPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	network, err := store.CreateNetwork("fixture")
	if err != nil {
		t.Fatal(err)
	}
	var self string
	for i, name := range []string{"alice", "bob"} {
		key := make([]byte, 32)
		key[0] = byte(i + 1)
		node, err := store.Join(netmap.JoinRequest{Network: network.ID, Name: name, PubKey: base64.StdEncoding.EncodeToString(key)})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.ApproveNode(node.NodeID); err != nil {
			t.Fatal(err)
		}
		self = node.NodeID
	}
	app, _, err := NewServer(store, nil, "http://control.invalid", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	app.sessions["fixture-session"] = time.Now().Add(time.Hour)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	policy := &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "*", Dst: "10.9.0.0/24"}}}
	for _, body := range []string{
		`{"network_id":%q}`,
		`{"network_id":%q,"ruless":[]}`,
		`{"network_id":%q,"rules":[],"rules":[{"src":"*","dst":"*"}]}`,
		`{"network_id":%q,"rules":[],"RULES":[{"src":"*","dst":"*"}]}`,
		`{"network_id":%q,"RULES":[]}`,
		`{"network_id":%q,"rules":[]} {}`,
		`{"network_id":%q,"rules":[],"network_id":"fixture"}`,
		`{"network_id":%q,"rules":[{"src":"alice","src":"*","dst":"*"}]}`,
		`{"network_id":%q,"rules":[{"src":"*","dst":"*","deny":true}]}`,
		`{"network_id":%q,"rules":[{"SRC":"*","DST":"*"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			if err := store.SetACL(network.ID, policy); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			version := store.Version()
			request, err := http.NewRequest(http.MethodPut, server.URL+"/admin/api/acl", strings.NewReader(fmt.Sprintf(body, network.ID)))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.AddCookie(&http.Cookie{Name: "mnet_admin", Value: "fixture-session"})
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				t.Error("ambiguous policy write accepted", response.StatusCode)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || store.Version() != version {
				t.Error("rejected policy request changed saved state or woke maps")
			}
			m, err := store.Map(self)
			if err != nil || len(m.Peers) != 0 {
				t.Error("malformed policy reopened peer visibility", err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			store = reopened
			app.store = reopened
			m, err = reopened.Map(self)
			if err != nil || len(m.Peers) != 0 {
				t.Error("malformed policy persisted reopened visibility", err)
			}
		})
	}
}

func TestACLRequestExactShapeAndClearCompatibility(t *testing.T) {
	for _, body := range []string{
		`{"network_id":"fixture","rules":[]}`,
		`{"rules":null,"network_id":"fixture"}`,
		` {"rules":[{"dst":"*","comment":"saved operator note","src":"alice"}],"network_id":"fixture"} `,
	} {
		got, err := decodeACLRequest(json.NewDecoder(strings.NewReader(body)))
		if err != nil || got.NetworkID != "fixture" {
			t.Fatalf("valid policy rejected: %v", err)
		}
		if strings.Contains(body, "alice") && (len(got.Rules) != 1 || got.Rules[0].Src != "alice" || got.Rules[0].Dst != "*" || got.Rules[0].Comment != "saved operator note") {
			t.Fatal("rule changed during decoding")
		}
	}
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"network_id":null,"rules":[]}`, `{"network_id":"fixture","rules":{}}`,
		`{"network_id":"fixture","rules":[null]}`, `{"network_id":"fixture","rules":[[]]}`,
		`{"network_id":"fixture","rules":[{"src":"*","dst":"*","comment":null}]}`,
		`{"network_id":"fixture","rules":[{"src":"*","dst":"*","comment":"a","comment":"b"}]}`,
		`{"network_id":"fixture","rules":[{"src":null,"dst":"*"}]}`,
		`{"network_id":"fixture","rules":[{"src":1,"dst":"*"}]}`,
		`{"network_id":"fixture","rules":[{"src":"*"}]}`,
		`{"network_id":"fixture","rules":[]} true`,
		`{"network_id":"fixture","rules":[],"network_\u0069d":"other"}`,
	} {
		if _, err := decodeACLRequest(json.NewDecoder(strings.NewReader(body))); err == nil {
			t.Errorf("malformed policy accepted: %s", body)
		}
	}
}
