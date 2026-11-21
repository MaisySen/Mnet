//go:build multies_native_fixture

package control

// Independent disposable client process. It never calls Store or server methods;
// it uses actual pending journal, own-key proof and HTTP transports throughout.
import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fintex.au/mnet/internal/controlclient"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	proof "fintex.au/mnet/internal/enrollmentproof"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	source "fintex.au/mnet/internal/enrollmentsource"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStandaloneInstalledTCPPeerFixture(t *testing.T) {
	dir := os.Getenv("MNET_TEST_STANDALONE_INSTALLED_DIR")
	if dir == "" {
		t.Skip("explicit external coordinator required")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "client-config.json"))
	if e != nil {
		t.Fatal(e)
	}
	var policyConfig registrarWorkflowConfig
	if json.Unmarshal(raw, &policyConfig) != nil {
		t.Fatal("invalid coordinator config")
	}
	raw, _ = json.Marshal(policyConfig)
	_, policy, e := parseRegistrarWorkflow(raw)
	if e != nil {
		t.Fatal(e)
	}
	cfg, ok := pending.SourceConfigFor(policy)
	if !ok {
		t.Fatal("policy")
	}
	current := func() pending.SourceConfig { return cfg }
	limits := registrar.WorkflowLimits{Timeout: 3 * time.Second, MaxChallengeLifetime: time.Duration(policyConfig.ChallengeMS) * time.Millisecond, MaxReserveLifetime: time.Minute, Timing: contract.FixtureTiming{MaxObservationAgeMS: int64(policyConfig.MaxObservationAgeMS), MaxLeaseMS: int64(policyConfig.MaxLeaseMS), ClockUncertaintyMS: int64(policyConfig.ClockUncertaintyMS)}}
	var clients []*controlclient.RegistrarClient

	var expiry int64
	var transport *registrarTransportFixture
	for i := 0; i < 1; i++ {
		private, e := ecdh.X25519().GenerateKey(rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		var key [32]byte
		copy(key[:], private.Bytes())
		journal, e := pending.Open(filepath.Join(dir, fmt.Sprintf("client-%d", i)))
		if e != nil {
			t.Fatal(e)
		}
		defer journal.Close()
		launch, e := journal.Begin(base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()), "linux", "http://127.0.0.1:34567/mnet/enrollment/callback", policy, time.Now(), time.Now().Add(time.Minute))
		if e != nil {
			t.Fatal(e)
		}
		public, _ := json.Marshal(launch)
		if e = writeRegistrarFixtureAtomic(dir, fmt.Sprintf("client-%d-launch.json", i), public); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(30 * time.Second)
		var code struct {
			Code string `json:"code"`
		}
		for {
			raw, e = os.ReadFile(filepath.Join(dir, fmt.Sprintf("client-%d-code.json", i)))
			if e == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("approval timeout")
			}
			time.Sleep(5 * time.Millisecond)
		}
		if json.Unmarshal(raw, &code) != nil {
			t.Fatal("code input")
		}
		if e = journal.RecordCallback(launch.AttemptID, launch.State, code.Code, time.Now()); e != nil {
			t.Fatal(e)
		}
		if _, e = journal.ExchangeSaved(context.Background(), current, time.Now, source.Limits{Timeout: 3 * time.Second, MaxConnections: 1}); e != nil {
			t.Fatal("own PKCE", e)
		}
		if _, e = journal.BeginWorkflowDATA(context.Background(), current, limits); e != nil {
			t.Fatal("HTTP begin", e)
		}
		wire, e := journal.ProveWorkflowOwned(context.Background(), current, func(a *pending.WorkflowKeyAdmission) (*proof.Native, error) {
			return a.RespondProof(func(c contract.Challenge, b contract.Binding) (*proof.Native, error) {
				return proof.Respond(key, c, b, time.Now(), limits.MaxChallengeLifetime)
			})
		})
		if e != nil {
			t.Fatal("owned proof", e)
		}
		var doc contract.NativeCredentialDocument
		if e = journal.FinalizeWorkflow(wire, registrarContractDigest(11), registrarContractDigest(12), cfg, func(d contract.NativeCredentialDocument) error { doc = d; return nil }); e != nil {
			t.Fatal("journal finalization", e)
		}
		client, e := controlclient.NewRegistrarClient(policyConfig.Policy.RegistrarOrigin, doc, controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 32})
		if e != nil {
			t.Fatal(e)
		}
		defer client.Close()
		clients = append(clients, client)

		if wire.Receipt.AccessExpiresAtMS > expiry {
			expiry = wire.Receipt.AccessExpiresAtMS
		}
		m, e := client.CurrentMap(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		transport = newRegistrarTCPTransportFixture(t, key, m.Self.IPv4)
		clear(key[:])
		if e = client.RegistrarEndpoints(context.Background(), []string{transport.endpoint()}); e != nil {
			t.Fatal("endpoints", e)
		}
		m, e = client.CurrentMap(context.Background())
		if e != nil || m.Self.ID != wire.Receipt.NodeID {
			t.Fatal("own current map", e)
		}
	}
	ready, _ := json.Marshal(map[string]any{"committedAtMS": time.Now().UnixMilli(), "accessExpiresAtMS": expiry})
	if e := writeRegistrarFixtureAtomic(dir, "installed-peers-ready", ready); e != nil {
		t.Fatal(e)
	}
	wait := time.Now().Add(8 * time.Second)
	for {
		if _, e := os.Stat(filepath.Join(dir, "installed-saved")); e == nil {
			break
		}
		if time.Now().After(wait) {
			t.Fatal("installed save did not finish in original peer interval")
		}
		time.Sleep(5 * time.Millisecond)
	}
	m, e := clients[0].CurrentMap(context.Background())
	if e != nil || len(m.Peers) != 1 {
		t.Fatal("saved installed peer missing", e)
	}
	peer := m.Peers[0]
	public, e := base64.StdEncoding.Strict().DecodeString(peer.PubKey)
	if e != nil || len(public) != 32 {
		t.Fatal("current enrolled key invalid")
	}
	// Configure the enrolled public identity BEFORE native sends its first
	// handshake. Endpoint is learned only from an authenticated incoming packet;
	// no native endpoint is invented or taken from the coordinator.
	if e = transport.dev.IpcSetOperation(strings.NewReader(fmt.Sprintf("public_key=%s\nallowed_ip=%s/32\n", hex.EncodeToString(public), peer.IPv4))); e != nil {
		t.Fatal(e)
	}
	if e = writeRegistrarFixtureAtomic(dir, "counterpart-configured", []byte("authorized current installed peer configured before first handshake")); e != nil {
		t.Fatal(e)
	}
	for attempts := 0; attempts < 26; attempts++ {
		m, e := clients[0].CurrentMap(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("counterpart map peers=%d", len(m.Peers))
		if len(m.Peers) == 1 {
			t.Logf("native endpoints=%v approved=%v expiry=%v", m.Peers[0].Endpoints, m.Peers[0].Approved, m.Peers[0].KeyExpiry)
			transport.tick(t, m, m.Peers[0])
			if transport.received > 0 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !transport.sent || transport.received == 0 {
		t.Fatal("actual encrypted TCP exchange not observed")
	}
	// Check exact original interval, never extend it for test execution.
	if e = writeRegistrarFixtureAtomic(dir, "clients-current.json", []byte(`{"installedTCPVerified":true,"journalOwnedProof":true,"counterpartNativeStateInstalled":false,"inputAuthorized":false}`)); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Until(time.UnixMilli(expiry)) + 20*time.Millisecond)
	for _, client := range clients {
		if _, e = client.CurrentMap(context.Background()); e == nil {
			t.Fatal("expired credential served current map")
		}
	}
	if e = writeRegistrarFixtureAtomic(dir, "clients-ended.json", []byte(`{"originalExpiryRefused":true,"renewalImplemented":false}`)); e != nil {
		t.Fatal(e)
	}

}
