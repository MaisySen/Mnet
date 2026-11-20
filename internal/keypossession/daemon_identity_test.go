package keypossession_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/keypossession"
)

// Test only: a real durable daemon WireGuard identity can answer the isolated
// experiment. This adapter is not installed in the daemon or local API. No
// private key is printed, transmitted, returned by HTTP or persisted elsewhere.
func TestActualPreparedDaemonIdentityAgreesWithoutEnrollment(t *testing.T) {
	dir := t.TempDir()
	d, err := daemon.New(daemon.Config{StateDir: dir})
	if err != nil {
		t.Fatal("local daemon fixture failed")
	}
	defer d.Close()
	public, err := d.PrepareIdentity()
	if err != nil {
		t.Fatal("private local preparation failed")
	}
	state := d.State()
	priv, err := base64.StdEncoding.Strict().DecodeString(state.PrivateKey)
	if err != nil || len(priv) != 32 {
		t.Fatal("invalid fixture private identity")
	}
	defer clear(priv)
	pub, err := base64.StdEncoding.Strict().DecodeString(public)
	if err != nil || len(pub) != 32 {
		t.Fatal("invalid fixture public identity")
	}
	var private, publicKey [32]byte
	copy(private[:], priv)
	defer clear(private[:])
	copy(publicKey[:], pub)
	b := keypossession.Binding{RegistrarOrigin: "https://fixture-registrar.test", Audience: "fixture-enrollment", Issuer: "https://fixture-multies.test", Subject: "fixture-person", PersonSessionID: "fixture-session", WorkspaceID: "fixture-workspace", NetworkID: "fixture-network", AuthorizationID: "fixture-decision-only", DevicePublicKey: publicKey}
	now := time.Now()
	pending, challenge, err := keypossession.New(b, now)
	if err != nil {
		t.Fatal("challenge fixture failed")
	}
	defer pending.Cancel()
	response, err := keypossession.Respond(private, challenge, b, now)
	if err != nil {
		t.Fatal("real WireGuard identity could not answer agreement proof")
	}
	if pending.Verify(challenge, response, now) != nil {
		t.Fatal("actual identity response refused")
	}
	after := d.State()
	if after.PrivateKey != state.PrivateKey || after.PublicKey != public {
		t.Fatal("proof changed durable local identity")
	}
	if d.Status().Status != "down" || after.Server != "" || after.Network != "" || after.NodeID != "" || after.NodeToken != "" || after.AuthKey != "" {
		t.Fatal("proof enrolled or started a network")
	}
	reloaded, err := daemon.LoadState(dir)
	if err != nil || reloaded.PublicKey != public || reloaded.PrivateKey != state.PrivateKey {
		t.Fatal("durable identity changed")
	}
	info, err := os.Stat(filepath.Join(dir, "state.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("identity no longer private")
	}
}
