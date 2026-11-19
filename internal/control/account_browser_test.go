package control_test

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestAccountRealChrome(t *testing.T) {
	if os.Getenv("MNET_ACCOUNT_BROWSER") != "1" {
		t.Skip("isolated Chrome proof is opt-in")
	}
	fake := newFakeOIDC(t, false)
	ts, pw, store := testEnvOIDCStore(t, fake)
	admin := adminLogin(t, ts.URL, pw)
	_, body := post(t, ts.URL+"/admin/api/networks", map[string]string{"name": "account-browser"}, map[string]string{"Cookie": admin.String()})
	var network netmap.Network
	json.Unmarshal(body, &network)
	joined, err := store.Join(netmap.JoinRequest{Network: network.ID, Name: "personal-laptop", PubKey: fakePubKey(0x65)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveNodeIdentity(joined.NodeID, "display@fintex.au", fake.srv.URL, "user-1"); err != nil {
		t.Fatal(err)
	}
	// Multiple owned identities plus look-alike outsiders exercise the real
	// HTTP response and DOM, not only a mocked table response.
	other, err := store.CreateNetwork("other-account-network")
	if err != nil {
		t.Fatal(err)
	}
	for i, fixture := range []struct {
		name, network, issuer, subject string
		expire                         bool
	}{
		{"personal-phone", network.ID, fake.srv.URL, "user-1", false},
		{"same-email-outsider", network.ID, fake.srv.URL, "different-user", false},
		{"other-network-device", other.ID, fake.srv.URL, "user-1", false},
		{"other-issuer-device", network.ID, "https://different-issuer.example", "user-1", false},
		{"expired-owned-device", network.ID, fake.srv.URL, "user-1", true},
	} {
		node, err := store.Join(netmap.JoinRequest{Network: fixture.network, Name: fixture.name, PubKey: fakePubKey(byte(0x66 + i))})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.ApproveNodeIdentity(node.NodeID, "display@fintex.au", fixture.issuer, fixture.subject); err != nil {
			t.Fatal(err)
		}
		if fixture.expire {
			if err := store.ExpireNode(node.NodeID); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A separate network avoids changing the account's two-device scope fixture.
	pending, err := store.Join(netmap.JoinRequest{Network: other.ID, Name: "approval-browser-device", PubKey: fakePubKey(0x77)})
	if err != nil {
		t.Fatal(err)
	}
	approvalURL := ts.URL + "/oidc/start?" + url.Values{"node_id": {pending.NodeID}, "token": {pending.NodeToken}}.Encode()
	cmd := exec.Command("node", "../../scripts/account-validation.mjs")
	cmd.Env = append(os.Environ(), "MNET_ACCOUNT_BASE="+ts.URL, "MNET_ACCOUNT_NETWORK="+network.ID, "MNET_ACCOUNT_FIXTURE_PASSWORD="+pw, "MNET_ACCOUNT_FIXTURE_ISSUER="+fake.srv.URL, "MNET_DEVICE_APPROVAL_URL="+approvalURL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real Chrome account proof failed: %v\n%s", err, output)
	}
	t.Log(string(output))
}
