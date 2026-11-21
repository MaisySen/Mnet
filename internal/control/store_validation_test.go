package control

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestStoreRejectsNullShapeWithoutReplacingFile(t *testing.T) {
	for _, body := range []string{"", " \n", "null", "[]", "true", `{"networks":null}`, `{"nodes":null}`, `{"auth_keys":null}`, `{"node_tokens":null}`, `{"next_ip":null}`, `{"networks":{"secret-record-marker":null}}`, `{"nodes":{"secret-record-marker":null}}`, `{"auth_keys":{"secret-record-marker":null}}`} {
		t.Run(body, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			st, err := openTestStore(t, path)
			if err == nil || st != nil {
				t.Fatal("corrupt shape accepted")
			}
			if strings.Contains(err.Error(), "secret-record-marker") {
				t.Fatal("error disclosed record identifier")
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(after, []byte(body)) {
				t.Fatal("rejected state file modified")
			}
		})
	}
}

func TestOldObjectStateWithoutNewFieldsRemainsUsable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"unknown_future_metadata":{"preserve_compatibility":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	network, err := st.CreateNetwork("legacy")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := st.Join(netmap.JoinRequest{Network: network.ID, Name: "device", PubKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AuthorizeNode(joined.NodeID, joined.NodeToken); err != nil {
		t.Fatal("credential lost on reload", err)
	}
	if _, known := st.NodeApprovalIdentity(joined.NodeID); known {
		t.Fatal("legacy shape inferred identity")
	}
}
