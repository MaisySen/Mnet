//go:build multies_native_fixture

package control

// Independent disposable client process. It never calls Store or server methods;
// it uses actual pending journal, own-key proof and HTTP transports throughout.
import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
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
	"testing"
	"time"
)

func TestDisposableRegistrarExternalClientFixture(t *testing.T) {
	dir := os.Getenv("MNET_TEST_EXTERNAL_REGISTRAR_CLIENT_DIR")
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
	var ids []string
	var expiry int64
	for i := 0; i < 2; i++ {
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
		clear(key[:])
		if e != nil {
			t.Fatal("owned proof", e)
		}
		var doc contract.NativeCredentialDocument
		if e = journal.FinalizeWorkflow(wire, registrarContractDigest(11), registrarContractDigest(12), cfg, func(d contract.NativeCredentialDocument) error { doc = d; return nil }); e != nil {
			t.Fatal("journal finalization", e)
		}
		client, e := controlclient.NewRegistrarClient(policyConfig.Policy.RegistrarOrigin, doc, controlclient.RegistrarLimits{Timeout: 3 * time.Second, MaxConcurrent: 1, MaxRequests: 6})
		if e != nil {
			t.Fatal(e)
		}
		defer client.Close()
		clients = append(clients, client)
		ids = append(ids, wire.Receipt.NodeID)
		if wire.Receipt.AccessExpiresAtMS > expiry {
			expiry = wire.Receipt.AccessExpiresAtMS
		}
		if e = client.RegistrarEndpoints(context.Background(), []string{fmt.Sprintf("127.0.0.1:%d", 41000+i)}); e != nil {
			t.Fatal("endpoints", e)
		}
		m, e := client.CurrentMap(context.Background())
		if e != nil || m.Self.ID != wire.Receipt.NodeID {
			t.Fatal("own current map", e)
		}
	}
	for i, client := range clients {
		m, e := client.CurrentMap(context.Background())
		if e != nil || len(m.Peers) != 1 || m.Peers[0].ID != ids[1-i] {
			t.Fatal("bidirectional actual peers", e)
		}
	}
	// Check exact original interval, never extend it for test execution.
	if e = writeRegistrarFixtureAtomic(dir, "clients-current.json", []byte(`{"twoActualPeers":true,"journalOwnedProof":true,"nativeStateInstalled":false,"inputAuthorized":false}`)); e != nil {
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
