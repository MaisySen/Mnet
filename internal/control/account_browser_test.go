package control_test

import (
	"encoding/json"
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
	if err := store.SetNetworkMember(network.ID, fake.srv.URL, "user-1", true); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "../../scripts/account-validation.mjs")
	cmd.Env = append(os.Environ(), "MNET_ACCOUNT_BASE="+ts.URL, "MNET_ACCOUNT_NETWORK="+network.ID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real Chrome account proof failed: %v\n%s", err, output)
	}
	t.Log(string(output))
}
