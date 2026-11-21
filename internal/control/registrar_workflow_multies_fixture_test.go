//go:build multies_native_fixture

package control

// Actual Multies app/SQLite coordinator + real source transports + private
// workflow. Native-side key/secret here are DISPOSABLE TEST-owned values, not a
// daemon key caller, native credential installation or registered bootstrap API.
import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	proof "fintex.au/mnet/internal/enrollmentproof"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	source "fintex.au/mnet/internal/enrollmentsource"
)

func TestActualMultiesRegistrarWorkflowFixture(t *testing.T) {
	dir := os.Getenv("MNET_WORKFLOW_FIXTURE_DIR")
	if dir == "" {
		t.Skip("explicit disposable coordinator required")
	}
	s, err := Open(filepath.Join(dir, "control", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	// Unregistered TEST bridge drives actual journal-owned exact-origin channel.
	var peerSecret string
	var bridgeMu sync.Mutex
	getPeerSecret := func() string { bridgeMu.Lock(); defer bridgeMu.Unlock(); return peerSecret }
	bridge := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimPrefix(r.Header.Get("Authorization"), registrar.WorkflowScheme)
		if secret == r.Header.Get("Authorization") {
			w.WriteHeader(403)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, contract.ProofWireMaxBytes+1))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case registrar.WorkflowPrefix + "begin":
			input, err := contract.DecodeWorkflowBeginWire(raw, "fixture-registrar")
			if err != nil {
				w.WriteHeader(400)
				return
			}
			c, err := s.beginRegistrarEnrollment(r.Context(), input.Request, secret)
			if err != nil {
				w.WriteHeader(403)
				return
			}
			bridgeMu.Lock()
			peerSecret = secret
			bridgeMu.Unlock()
			json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
		case registrar.WorkflowPrefix + "submit":
			input, err := contract.DecodeWorkflowSubmitWire(raw)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			mac, _ := base64.RawURLEncoding.DecodeString(input.ResponseMAC)
			var response [32]byte
			copy(response[:], mac)
			result, err := s.submitRegistrarEnrollment(r.Context(), input.AttemptID, input.ChallengeID, secret, response)
			if err != nil {
				w.WriteHeader(403)
				return
			}
			json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: result.History.Receipt, ReceiptDigest: result.History.Digest, ConfirmationMAC: base64.RawURLEncoding.EncodeToString(result.Confirmation[:])})
		default:
			w.WriteHeader(404)
		}
	}))
	defer bridge.Close()
	// httptest uses its standard embedded localhost certificate. Merge that with
	// the coordinator's freshly generated source certificate before first TLS.
	roots, err := os.ReadFile(os.Getenv("SSL_CERT_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	roots = append(roots, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: bridge.Certificate().Raw})...)
	combined := filepath.Join(dir, "combined-fixture-roots.pem")
	os.WriteFile(combined, roots, 0600)
	t.Setenv("SSL_CERT_FILE", combined)
	network, err := s.CreateNetwork("development-workflow")
	if err != nil {
		t.Fatal(err)
	}
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var key [32]byte
	copy(key[:], private.Bytes())
	defer clear(key[:])
	opaque := func() string {
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw[:])
	}
	secret, verifier := opaque(), opaque()
	hash := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return base64.RawURLEncoding.EncodeToString(sum[:])
	}
	launch := contract.SourceLaunch{Version: 1, Purpose: "mnet-enrollment", AttemptID: opaque(), DevicePublicKey: base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()), CredentialDigest: hash(secret), Challenge: hash(verifier), State: opaque(), RedirectURI: "http://127.0.0.1:34567/mnet/enrollment/callback", Platform: "linux"}
	// Private durable fixture presave and exchange marker precede source I/O.
	presaved, _ := json.Marshal(struct {
		Secret, Verifier string
		Private          [32]byte
	}{secret, verifier, key})
	if err := writeWorkflowFixturePrivate(filepath.Join(dir, "presaved-test-only.json"), presaved); err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(struct {
		Launch          contract.SourceLaunch `json:"launch"`
		NetworkID       string                `json:"networkId"`
		RegistrarOrigin string                `json:"registrarOrigin"`
	}{launch, network.ID, bridge.URL})
	if err := os.WriteFile(filepath.Join(dir, "launch.json"), public, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(40 * time.Second)
	var raw []byte
	for {
		raw, err = os.ReadFile(filepath.Join(dir, "coordinator.json"))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("coordinator timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var input struct {
		Mode     string                  `json:"mode"`
		Code     string                  `json:"code"`
		Sources  registrarSourceConfig   `json:"sources"`
		Workflow registrarWorkflowConfig `json:"workflow"`
	}
	if json.Unmarshal(raw, &input) != nil {
		t.Fatal("invalid coordinator input")
	}
	policyRaw, _ := json.Marshal(input.Workflow)
	_, policy, err := parseRegistrarWorkflow(policyRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Sources.Sources) != 1 {
		t.Fatal("missing independent source config")
	}
	for name, v := range map[string]any{".registrar-sources-v1.json": input.Sources, ".registrar-workflow-v1.json": input.Workflow} {
		raw, _ := json.Marshal(v)
		if err := os.WriteFile(s.ownedPath+name, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	native, err := source.NewNative(input.Workflow.Policy.SourceOrigin, input.Workflow.Policy.PrincipalAuthority, source.Limits{Timeout: 3 * time.Second, MaxConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer native.CloseIdleConnections()
	if err := writeWorkflowFixturePrivate(filepath.Join(dir, "exchange-may-have-started"), []byte("test-only same attempt marker")); err != nil {
		t.Fatal(err)
	}
	own, err := native.Exchange(context.Background(), launch, input.Workflow.Policy.SourceOrigin, input.Workflow.Policy.PrincipalAuthority, input.Code, verifier, policy.SourceMappings(), policy.SourceMappings(), time.Now())
	if err != nil {
		t.Fatal("actual own-PKCE failed", err)
	}
	if !policy.AllowsObservedDATA(own.Mapping, own.Provenance.PrincipalAuthority, own.Provenance.PrincipalNamespace, own.Provenance.PrincipalID, input.Workflow.Policy.ServiceID) {
		t.Fatal("own source outside independent native policy")
	}
	req := contract.SourceReserveRequest{Ticket: own.Ticket, HandoffID: own.ID, ReserveID: opaque(), Launch: launch, Mapping: own.Mapping}
	c, err := s.beginRegistrarEnrollment(context.Background(), req, secret)
	if input.Mode == "source-denied" {
		if err == nil || len(s.data.Nodes) != 0 || s.data.Registrar != nil {
			t.Fatal("denied actual source granted node")
		}
		os.WriteFile(filepath.Join(dir, "outcome.json"), []byte(`{"sourceDenied":true,"nodeCreated":false,"inputAuthorized":false}`), 0600)
		return
	}
	if err != nil {
		t.Fatal("actual source -> owned challenge", err)
	}
	descriptor, _ := contract.SourceDescriptorDigest(own, input.Workflow.Policy.ServiceID, req.ReserveID)
	// Independently known identity/launch/mapping from own-PKCE + installed policy,
	// NOT c.Binding echoed as expected. Only registrar times are selected after
	// independent finite timing checks, inside this explicit test harness.
	p, m := own.Provenance, own.Mapping
	expected := contract.Binding{PrincipalAuthority: p.PrincipalAuthority, PrincipalNamespace: p.PrincipalNamespace, PrincipalID: p.PrincipalID, SessionBinding: p.SessionBinding, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, ServiceID: input.Workflow.Policy.ServiceID, WorkspaceID: m.WorkspaceID, NetworkID: m.NetworkID, MappingRevision: m.MappingRevision, HandoffID: own.ID, AttemptID: launch.AttemptID, ReserveID: req.ReserveID, DescriptorDigest: descriptor, DescriptorEncodingVersion: 1, DevicePublicKey: launch.DevicePublicKey, CredentialDigest: launch.CredentialDigest, CheckedAtMS: c.Binding.CheckedAtMS, ReserveDeadlineMS: c.Binding.ReserveDeadlineMS, AccessUntilMS: c.Binding.AccessUntilMS}
	if c.Binding != expected || contract.CheckFixtureTime(c, time.Now().UnixMilli(), contract.FixtureTiming{MaxObservationAgeMS: int64(input.Workflow.MaxObservationAgeMS), MaxLeaseMS: int64(input.Workflow.MaxLeaseMS), ClockUncertaintyMS: int64(input.Workflow.ClockUncertaintyMS)}) != nil {
		t.Fatal("server outside independently checked context")
	}
	n, err := proof.Respond(key, c, expected, time.Now(), time.Duration(input.Workflow.ChallengeMS)*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	response, err := n.Response(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.submitRegistrarEnrollment(context.Background(), launch.AttemptID, c.ID, secret, response)
	if err != nil {
		t.Fatal("actual atomic commit", err)
	}
	if n.Confirm(result.Confirmation, time.Now()) != nil {
		t.Fatal("confirmation failed")
	}
	if len(s.data.Nodes) != 1 || len(s.data.NodeTokens) != 0 || len(s.data.Registrar.Records) != 1 || len(s.data.Registrar.ConsumedProofs) != 1 {
		t.Fatal("incomplete/mixed atomic image")
	}
	s.Close()
	s, err = Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := s.registrarHistoricalReceipt(c, secret)
	if err != nil || history != result.History {
		t.Fatal("whole real commit missing after reopen", err)
	}
	r := s.registrarNodeLocked(history.Receipt.NodeID)
	ref := registrarCredentialReference{NodeID: r.NodeID, NetworkID: r.NetworkID, DevicePublicKey: r.DevicePublicKey, Scope: r.Scope, ReceiptID: r.ReceiptID, ReceiptDigest: r.ReceiptDigest, DescriptorDigest: r.DescriptorDigest, TranscriptDigest: r.Consumption.TranscriptDigest}
	if err := s.setRegistrarCredentialEndpoints(ref, secret, []string{"192.0.2.77:51820"}); err != nil {
		t.Fatal(err)
	}
	current, err := s.registrarCredentialMap(ref, secret)
	if err != nil || current.Self.ID != r.NodeID || len(current.Self.Endpoints) != 1 || current.RelayURL != "" {
		t.Fatal("new current credential not usable", err)
	}
	// A second independently approved disposable device exercises actual peer
	// delivery, not an injected peer fixture or a claim of direct transport.
	peerPrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var peerKey [32]byte
	copy(peerKey[:], peerPrivate.Bytes())
	defer clear(peerKey[:])
	peerJournal, err := pending.Open(filepath.Join(dir, "peer-pending"))
	if err != nil {
		t.Fatal(err)
	}
	defer peerJournal.Close()
	peerLaunch, err := peerJournal.Begin(base64.RawURLEncoding.EncodeToString(peerPrivate.PublicKey().Bytes()), "linux", "http://127.0.0.1:34567/mnet/enrollment/callback", policy, time.Now(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	peerPublic, _ := json.Marshal(peerLaunch)
	if err := os.WriteFile(filepath.Join(dir, "peer-launch.json"), peerPublic, 0600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(30 * time.Second)
	for {
		raw, err = os.ReadFile(filepath.Join(dir, "peer-code.json"))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second device consent timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var peerInput struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &peerInput) != nil {
		t.Fatal("invalid peer callback")
	}
	if err := peerJournal.RecordCallback(peerLaunch.AttemptID, peerLaunch.State, peerInput.Code, time.Now()); err != nil {
		t.Fatal(err)
	}
	currentConfig, ok := pending.SourceConfigFor(policy)
	if !ok {
		t.Fatal("missing peer current policy")
	}
	currentPeer := func() pending.SourceConfig { return currentConfig }
	peerOwn, err := peerJournal.ExchangeSaved(context.Background(), currentPeer, time.Now, source.Limits{Timeout: 3 * time.Second, MaxConnections: 1})
	if err != nil {
		t.Fatal("real peer journal own exchange", err)
	}
	limits := registrar.WorkflowLimits{Timeout: 3 * time.Second, MaxChallengeLifetime: time.Duration(input.Workflow.ChallengeMS) * time.Millisecond, MaxReserveLifetime: time.Minute, Timing: contract.FixtureTiming{MaxObservationAgeMS: int64(input.Workflow.MaxObservationAgeMS), MaxLeaseMS: int64(input.Workflow.MaxLeaseMS), ClockUncertaintyMS: int64(input.Workflow.ClockUncertaintyMS)}}
	peerChallenge, err := peerJournal.BeginWorkflowDATA(context.Background(), currentPeer, limits)
	if err != nil {
		t.Fatal("journal exact-origin begin", err)
	}
	peerExpected := expected
	peerExpected.PrincipalAuthority = peerOwn.Provenance.PrincipalAuthority
	peerExpected.PrincipalNamespace = peerOwn.Provenance.PrincipalNamespace
	peerExpected.PrincipalID = peerOwn.Provenance.PrincipalID
	peerExpected.SessionBinding = peerOwn.Provenance.SessionBinding
	peerExpected.HandoffID = peerOwn.ID
	peerExpected.AttemptID = peerOwn.Launch.AttemptID
	peerExpected.DevicePublicKey = peerOwn.Launch.DevicePublicKey
	peerExpected.CredentialDigest = peerOwn.Launch.CredentialDigest
	peerExpected.ReserveID = peerChallenge.Binding.ReserveID
	peerExpected.DescriptorDigest, _ = contract.SourceDescriptorDigest(peerOwn, input.Workflow.Policy.ServiceID, peerExpected.ReserveID)
	peerExpected.CheckedAtMS = peerChallenge.Binding.CheckedAtMS
	peerExpected.ReserveDeadlineMS = peerChallenge.Binding.ReserveDeadlineMS
	peerExpected.AccessUntilMS = peerChallenge.Binding.AccessUntilMS
	// independently validate via own source/config helper, not response echo.
	if peerOwn.Mapping != own.Mapping || registrar.CheckWorkflowChallenge(peerChallenge, peerOwn, input.Workflow.Policy.ServiceID, peerExpected.ReserveID, limits, time.Now()) != nil {
		t.Fatal("peer independently checked source differs")
	}
	peerNative, err := proof.Respond(peerKey, peerChallenge, peerExpected, time.Now(), limits.MaxChallengeLifetime)
	if err != nil {
		t.Fatal(err)
	}
	defer peerNative.Close()
	peerResponse, err := peerNative.Response(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	peerWire, err := peerJournal.SubmitWorkflowDATA(context.Background(), peerChallenge.ID, peerResponse, currentPeer)
	if err != nil {
		t.Fatal("journal exact-origin commit delivery", err)
	}
	confirmationRaw, _ := base64.RawURLEncoding.DecodeString(peerWire.ConfirmationMAC)
	var peerConfirmation [32]byte
	copy(peerConfirmation[:], confirmationRaw)
	if peerNative.Confirm(peerConfirmation, time.Now()) != nil {
		t.Fatal("peer confirmation mismatch")
	}
	phase, err := peerJournal.Status()
	if err != nil || phase != "workflow-receipt-data" {
		t.Fatal("receipt returned without durable pin", err)
	}
	peerResult := registrarEnrollmentResult{History: registrarReceiptHistory{Receipt: peerWire.Receipt, Digest: peerWire.ReceiptDigest}, Confirmation: peerConfirmation}
	peerRecord := s.registrarNodeLocked(peerResult.History.Receipt.NodeID)
	peerRef := registrarCredentialReference{NodeID: peerRecord.NodeID, NetworkID: peerRecord.NetworkID, DevicePublicKey: peerRecord.DevicePublicKey, Scope: peerRecord.Scope, ReceiptID: peerRecord.ReceiptID, ReceiptDigest: peerRecord.ReceiptDigest, DescriptorDigest: peerRecord.DescriptorDigest, TranscriptDigest: peerRecord.Consumption.TranscriptDigest}
	current, err = s.registrarCredentialMap(ref, secret)
	if err != nil || len(current.Peers) != 1 || current.Peers[0].ID != peerRecord.NodeID {
		t.Fatal("first credential did not receive actual approved peer", err)
	}
	peerMap, err := s.registrarCredentialMap(peerRef, getPeerSecret())
	if err != nil || len(peerMap.Peers) != 1 || peerMap.Peers[0].ID != r.NodeID {
		t.Fatal("second credential did not receive first peer", err)
	}

	// Third device: separate actual daemon test process owns the retained native
	// State/key/callback. This process never receives its private key or file
	// credential; only the real expected registrar request presents the bearer.
	peerSavedSecret := getPeerSecret()
	nativeBinary := os.Getenv("MNET_NATIVE_WORKFLOW_BINARY")
	if nativeBinary == "" {
		t.Fatal("actual native fixture binary required")
	}
	nativeConfig, _ := json.Marshal(struct {
		Policy config.LocalConfigV2
		Limits registrar.WorkflowLimits
	}{input.Workflow.Policy, limits})
	if err := os.WriteFile(filepath.Join(dir, "native-workflow-config.json"), nativeConfig, 0600); err != nil {
		t.Fatal(err)
	}
	nativeCmd := exec.Command(nativeBinary, "-test.run=^TestActualMultiesNativeWorkflowFixture$", "-test.v", "-test.timeout=40s")
	nativeCmd.Env = append(os.Environ(), "MNET_NATIVE_OWNED_WORKFLOW_DIR="+dir)
	var nativeOutput bytes.Buffer
	nativeCmd.Stdout = &nativeOutput
	nativeCmd.Stderr = &nativeOutput
	if err := nativeCmd.Start(); err != nil {
		t.Fatal(err)
	}
	nativeWaited := false
	defer func() {
		if !nativeWaited {
			nativeCmd.Process.Kill()
			nativeCmd.Wait()
		}
	}()
	deadline = time.Now().Add(35 * time.Second)
	for {
		raw, err = os.ReadFile(filepath.Join(dir, "native-workflow-outcome.json"))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual native process timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := nativeCmd.Wait(); err != nil {
		nativeWaited = true
		t.Fatalf("actual native process failed: %v\n%s", err, nativeOutput.String())
	}
	nativeWaited = true
	var nativeOutcome struct {
		Receipt                 contract.WorkflowReceiptWire
		NativeOwnedKeyConfirmed bool
		NativeStateUnchanged    bool
	}
	if json.Unmarshal(raw, &nativeOutcome) != nil || !nativeOutcome.NativeOwnedKeyConfirmed || !nativeOutcome.NativeStateUnchanged {
		t.Fatal("missing actual daemon proof result")
	}
	nativeRecord := s.registrarNodeLocked(nativeOutcome.Receipt.Receipt.NodeID)
	if nativeRecord == nil || len(s.data.Nodes) != 3 {
		t.Fatal("actual daemon proof not atomically in Store")
	}
	nativeRef := registrarCredentialReference{NodeID: nativeRecord.NodeID, NetworkID: nativeRecord.NetworkID, DevicePublicKey: nativeRecord.DevicePublicKey, Scope: nativeRecord.Scope, ReceiptID: nativeRecord.ReceiptID, ReceiptDigest: nativeRecord.ReceiptDigest, DescriptorDigest: nativeRecord.DescriptorDigest, TranscriptDigest: nativeRecord.Consumption.TranscriptDigest}
	nativeSecret := getPeerSecret()
	for _, item := range []struct {
		ref    registrarCredentialReference
		secret string
	}{{ref, secret}, {peerRef, peerSavedSecret}, {nativeRef, nativeSecret}} {
		got, err := s.registrarCredentialMap(item.ref, item.secret)
		if err != nil || len(got.Peers) != 2 {
			t.Fatal("three actual approved peers not mutually usable", err)
		}
	}
	if err := s.DeleteNode(nativeRecord.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.registrarCredentialMap(nativeRef, nativeSecret); err == nil {
		t.Fatal("deleted daemon peer retained current authority")
	}
	bridgeMu.Lock()
	peerSecret = peerSavedSecret
	bridgeMu.Unlock()
	if err := s.DeleteNode(r.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.registrarCredentialMap(ref, secret); err == nil {
		t.Fatal("deleted node still usable")
	}
	if got, err := s.registrarHistoricalReceipt(c, secret); err != nil || got != history {
		t.Fatal("delete erased history", err)
	}
	peerMap, err = s.registrarCredentialMap(peerRef, getPeerSecret())
	if err != nil || len(peerMap.Peers) != 0 {
		t.Fatal("deleted peer remained in second map", err)
	}
	if err := s.DeleteNode(peerRecord.NodeID); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(map[string]any{"sourceDenied": false, "bidirectionalActualPeers": true, "nativeOwnedKeyConfirmed": true, "threeActualPeers": true, "nativeStateUnchanged": true, "journalWorkflowReceiptPinned": true, "nodeCreated": true, "nodeDeleted": true, "currentMapBeforeDelete": true, "legacyTokenCreated": false, "inputAuthorized": false, "receiptId": r.ReceiptID, "receiptDigest": r.ReceiptDigest, "reserveId": req.ReserveID, "descriptorDigest": descriptor, "handoffId": own.ID})
	if err := os.WriteFile(filepath.Join(dir, "outcome.json"), out, 0600); err != nil {
		t.Fatal(err)
	}
}

// Test-only first-write intent files, no production native-State writer. Both
// file and directory are synced before any matching one-shot source exchange.
func writeWorkflowFixturePrivate(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if n, err := f.Write(raw); err != nil || n != len(raw) {
		f.Close()
		if err != nil {
			return err
		}
		return io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
