//go:build multies_native_fixture

package control

// Actual Multies app/SQLite coordinator + real source transports + private
// workflow. Native-side key/secret here are DISPOSABLE TEST-owned values, not a
// daemon key caller, native credential installation or registered bootstrap API.
import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"golang.zx2c4.com/wireguard/tun/tuntest"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
	proof "fintex.au/mnet/internal/enrollmentproof"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	source "fintex.au/mnet/internal/enrollmentsource"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
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
	var recoveryMode atomic.Bool
	var historyFinalize atomic.Bool
	var preparedFinalize atomic.Bool
	var preparedHistoryFinalize atomic.Bool
	var transportMode atomic.Bool
	var relayMode atomic.Bool
	var relayIngress atomic.Pointer[http.Handler]
	relayHub := relay.NewHub()
	var workflowSubmits atomic.Int32
	var historyCalls atomic.Int32
	var mapResponseRefusals atomic.Int32
	var bridgeMu sync.Mutex
	getPeerSecret := func() string { bridgeMu.Lock(); defer bridgeMu.Unlock(); return peerSecret }
	var ingress atomic.Pointer[registrarIngress]
	bridge := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == registrarRelayPath {
			h := relayIngress.Load()
			if h == nil {
				w.WriteHeader(503)
				return
			}
			(*h).ServeHTTP(w, r)
			return
		}
		g := ingress.Load()
		if g == nil {
			w.WriteHeader(503)
			return
		}
		g.ServeHTTP(w, r)
	}))
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path == registrar.HistoryPath {
			historyCalls.Add(1)
			raw, err := io.ReadAll(io.LimitReader(r.Body, contract.ProofWireMaxBytes+1))
			input, decodeErr := contract.DecodeHistoryRequestWire(raw)
			secret := strings.TrimPrefix(r.Header.Get("Authorization"), registrar.HistoryScheme)
			if err != nil || decodeErr != nil || secret == r.Header.Get("Authorization") {
				w.WriteHeader(400)
				return
			}
			if recoveryMode.Load() && !historyFinalize.Load() {
				s.mu.Lock()
				var record *registrarRecord
				for _, candidate := range s.data.Registrar.Records {
					if candidate.Consumption != nil && candidate.Consumption.Challenge == input.Challenge {
						record = candidate
						break
					}
				}
				s.mu.Unlock()
				if record == nil {
					w.WriteHeader(403)
					return
				}
				if err := s.DeleteNode(record.NodeID); err != nil {
					w.WriteHeader(403)
					return
				}
			}
			history, err := s.registrarHistoricalReceipt(input.Challenge, secret)
			if err != nil {
				w.WriteHeader(403)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version: 1, Receipt: history.Receipt, ReceiptDigest: history.Digest})
			return
		}
		if r.URL.Path == "/api/mnet-node-experimental-v1/map" || r.URL.Path == "/api/mnet-node-experimental-v1/endpoints" {
			nativeSecret := strings.TrimPrefix(r.Header.Get("Authorization"), "Mnet-Node-Credential-V1 ")
			var req struct {
				Version       int                `json:"version"`
				Receipt       contract.ReceiptV2 `json:"receipt"`
				ReceiptDigest string             `json:"receipt_digest"`
				Endpoints     []string           `json:"endpoints"`
			}
			raw, err := io.ReadAll(io.LimitReader(r.Body, contract.ProofWireMaxBytes+1))
			if err != nil || len(raw) > contract.ProofWireMaxBytes || json.Unmarshal(raw, &req) != nil || req.Version != 1 || nativeSecret == r.Header.Get("Authorization") {
				w.WriteHeader(400)
				return
			}
			s.mu.Lock()
			record := s.registrarNodeLocked(req.Receipt.NodeID)
			if record == nil || record.ReceiptID != req.Receipt.ReceiptID || record.ReceiptDigest != req.ReceiptDigest {
				s.mu.Unlock()
				w.WriteHeader(403)
				return
			}
			ref := registrarCredentialReference{NodeID: record.NodeID, NetworkID: record.NetworkID, DevicePublicKey: record.DevicePublicKey, Scope: record.Scope, ReceiptID: record.ReceiptID, ReceiptDigest: record.ReceiptDigest, DescriptorDigest: record.DescriptorDigest, TranscriptDigest: record.Consumption.TranscriptDigest}
			s.mu.Unlock()

			if r.URL.Path == "/api/mnet-node-experimental-v1/endpoints" {
				if err := s.setRegistrarCredentialEndpoints(ref, nativeSecret, req.Endpoints); err != nil {
					w.WriteHeader(403)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(struct {
					Version       int    `json:"version"`
					ReceiptDigest string `json:"receipt_digest"`
					Updated       bool   `json:"updated"`
				}{1, req.ReceiptDigest, true})
				return
			}
			m, err := s.registrarCredentialMap(ref, nativeSecret)
			if err != nil {
				w.WriteHeader(403)
				return
			}
			if os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY") != "" && os.Getenv("MNET_DEVELOPMENT_SETUP_CASE") == "map-failure" {
				mapResponseRefusals.Add(1)
				w.WriteHeader(503)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(struct {
				Version       int         `json:"version"`
				ReceiptDigest string      `json:"receipt_digest"`
				Map           *netmap.Map `json:"map"`
			}{1, req.ReceiptDigest, m})
			return
		}
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

			if workflowSubmits.Add(1) == 2 && (recoveryMode.Load() || os.Getenv("MNET_DEVELOPMENT_SETUP_CASE") == "lost-response") {
				if len(s.data.Nodes) != 3 {
					t.Error("lost-submit third node not committed")
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return
			}
			json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: result.History.Receipt, ReceiptDigest: result.History.Digest, ConfirmationMAC: base64.RawURLEncoding.EncodeToString(result.Confirmation[:])})
		default:
			w.WriteHeader(404)
		}
	})
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
	gate, err := s.newRegistrarDevelopmentIngress(registrarIngressLimits{MaxRequests: 64, MaxConcurrent: 4, MaxBodyBytes: 16384, MaxStateBytes: 4 << 20, MaxEntries: 8, Timeout: 5 * time.Second}, inner)
	if err != nil {
		t.Fatal(err)
	}
	ingress.Store(gate)
	rh, relayErr := s.newRegistrarDevelopmentRelayIngress(gate, relayHub, 256)
	if relayErr != nil {
		t.Fatal(relayErr)
	}
	relayIngress.Store(&rh)
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
	if err := writeRegistrarFixtureAtomic(dir, "launch.json", public); err != nil {
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
	recoveryMode.Store(input.Mode == "history-recovery" || input.Mode == "history-finalize")
	historyFinalize.Store(input.Mode == "history-finalize")
	preparedFinalize.Store(input.Mode == "prepared-finalize" || input.Mode == "prepared-history-finalize")
	preparedHistoryFinalize.Store(input.Mode == "prepared-history-finalize")
	relayMode.Store(input.Mode == "relay-transport")
	transportMode.Store(input.Mode == "direct-transport" || relayMode.Load() || strings.HasPrefix(os.Getenv("MNET_DEVELOPMENT_SETUP_CASE"), "network-tcp"))
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

	networkCase := strings.HasPrefix(os.Getenv("MNET_DEVELOPMENT_SETUP_CASE"), "network-tcp")
	var earlyInstalled *exec.Cmd
	var nativeOutput bytes.Buffer
	nativeWaited := false
	limits := registrar.WorkflowLimits{Timeout: 3 * time.Second, MaxChallengeLifetime: time.Duration(input.Workflow.ChallengeMS) * time.Millisecond, MaxReserveLifetime: time.Minute, Timing: contract.FixtureTiming{MaxObservationAgeMS: int64(input.Workflow.MaxObservationAgeMS), MaxLeaseMS: int64(input.Workflow.MaxLeaseMS), ClockUncertaintyMS: int64(input.Workflow.ClockUncertaintyMS)}}
	if networkCase {
		installed := os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY")
		if installed == "" {
			t.Fatal("installed network binary required")
		}
		earlyInstalled = newInstalledDevelopmentFixtureCmd(t, dir, installed, input.Workflow.Policy, limits)
		earlyInstalled.Env = append(os.Environ(), "MNET_NATIVE_OWNED_WORKFLOW_DIR="+dir)
		earlyInstalled.Stderr = &nativeOutput
		if err := earlyInstalled.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if !nativeWaited {
				earlyInstalled.Process.Kill()
				earlyInstalled.Wait()
			}
		}()
		waitUntil := time.Now().Add(45 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(dir, "installed-consent-ready")); err == nil {
				break
			}
			if time.Now().After(waitUntil) {
				t.Fatal("installed consent readiness timeout")
			}
			time.Sleep(10 * time.Millisecond)
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
		writeRegistrarFixtureAtomic(dir, "outcome.json", []byte(`{"sourceDenied":true,"nodeCreated":false,"inputAuthorized":false}`))
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
	gate, err = s.newRegistrarDevelopmentIngress(registrarIngressLimits{MaxRequests: 64, MaxConcurrent: 4, MaxBodyBytes: 16384, MaxStateBytes: 4 << 20, MaxEntries: 8, Timeout: 5 * time.Second}, inner)
	if err != nil {
		t.Fatal(err)
	}
	ingress.Store(gate)
	rh, relayErr = s.newRegistrarDevelopmentRelayIngress(gate, relayHub, 256)
	if relayErr != nil {
		t.Fatal(relayErr)
	}
	relayIngress.Store(&rh)
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
	if err := writeRegistrarFixtureAtomic(dir, "peer-launch.json", peerPublic); err != nil {
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

	var transport *registrarTransportFixture
	if transportMode.Load() {
		if os.Getenv("MNET_DEVELOPMENT_NETWORK_TCP") == "1" {
			transport = newRegistrarTCPTransportFixture(t, key, current.Self.IPv4)
		} else {
			transport = newRegistrarTransportFixture(t, key)
		}
		endpoints := []string{transport.endpoint()}
		if relayMode.Load() {
			endpoints = nil
			transport.relayMode = true
		}
		if err := s.setRegistrarCredentialEndpoints(ref, secret, endpoints); err != nil {
			t.Fatal(err)
		}
		if relayMode.Load() {
			receipt, err := contract.ReceiptV2ForChallenge(r.Consumption.Challenge, r.NodeID, r.ReceiptID, r.CommittedAtMS)
			if err != nil {
				t.Fatal(err)
			}
			transport.connectRelay(t, bridge, contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: r.ReceiptDigest}, secret)
			if err := os.WriteFile(filepath.Join(dir, "transport-peer-id"), []byte(r.NodeID), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Third device: separate actual daemon test process owns the retained native
	// State/key/callback. This process never receives its private key or file
	// credential; only the real expected registrar request presents the bearer.
	if networkCase {
		if err := writeRegistrarFixtureAtomic(dir, "installed-peers-ready", []byte("fresh actual disposable counterpart committed")); err != nil {
			t.Fatal(err)
		}
	}
	peerSavedSecret := getPeerSecret()
	nativeBinary := os.Getenv("MNET_NATIVE_WORKFLOW_BINARY")
	if nativeBinary == "" {
		t.Fatal("actual native fixture binary required")
	}
	nativeConfig, _ := json.Marshal(struct {
		Policy                  config.LocalConfigV2
		Limits                  registrar.WorkflowLimits
		Recover                 bool
		FinalizeHistory         bool
		PreparedFinalize        bool
		PreparedHistoryFinalize bool
		Transport               bool
		RelayTransport          bool
	}{input.Workflow.Policy, limits, recoveryMode.Load(), historyFinalize.Load(), preparedFinalize.Load(), preparedHistoryFinalize.Load(), transportMode.Load(), relayMode.Load()})
	if err := writeRegistrarFixtureAtomic(dir, "native-workflow-config.json", nativeConfig); err != nil {
		t.Fatal(err)
	}
	nativeTimeout := "40s"
	nativeCmd := exec.Command(nativeBinary, "-test.run=^TestActualMultiesNativeWorkflowFixture$", "-test.v", "-test.timeout="+nativeTimeout)

	if installed := os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY"); installed != "" {
		if earlyInstalled != nil {
			nativeCmd = earlyInstalled
		} else {
			if input.Mode != "success" {
				t.Fatal("installed setup fixture only supports success mode")
			}
			nativeCmd = newInstalledDevelopmentFixtureCmd(t, dir, installed, input.Workflow.Policy, limits)
		}
	}
	nativeCmd.Env = append(os.Environ(), "MNET_NATIVE_OWNED_WORKFLOW_DIR="+dir)
	if os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY") == "" {
		nativeCmd.Stdout = &nativeOutput
	}
	nativeCmd.Stderr = &nativeOutput
	if earlyInstalled == nil {
		if err := nativeCmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		if !nativeWaited {
			nativeCmd.Process.Kill()
			nativeCmd.Wait()
		}
	}()
	deadline = time.Now().Add(35 * time.Second)
	var transportNativeID string
	var transportReceivedAtRevoke int
	var postRemovalBarrier bool
	var postRemovalCheckedAt time.Time
	var postRemovalVerified bool
	for {
		setupCase := os.Getenv("MNET_DEVELOPMENT_SETUP_CASE")
		if setupCase == "network-tcp-cancel" || setupCase == "network-tcp-expiry" {
			if _, err := os.Stat(filepath.Join(dir, "native-transport-ready")); err == nil && setupCase == "network-tcp-cancel" {
				_ = nativeCmd.Process.Signal(os.Interrupt)
			}
			if _, err := os.Stat(filepath.Join(dir, "installed-output-ended")); err == nil {
				err := nativeCmd.Wait()
				nativeWaited = true
				if err == nil && setupCase == "network-tcp-cancel" {
					t.Fatal("cancelled network proof unexpectedly succeeded")
				}
				if _, err := os.Stat(filepath.Join(dir, "native-transport-ready")); err != nil {
					t.Fatal("network stopped before actual TCP proof")
				}
				root := filepath.Join(dir, "installed-setup")
				state, err := os.ReadFile(filepath.Join(root, "native", "state.json"))
				if err != nil {
					t.Fatal(err)
				}
				var image map[string]any
				if json.Unmarshal(state, &image) != nil || image["native_enrollment_v1"] == nil || len(s.data.Nodes) != 3 {
					t.Fatal("stopped network lost saved credential or rolled back remote nodes")
				}
				if setupCase == "network-tcp-cancel" {
					if _, err := os.Stat(filepath.Join(root, "network-result.html")); !os.IsNotExist(err) {
						t.Fatal("cancelled network claimed confirmed peer withdrawal")
					}
				}
				if setupCase == "network-tcp-expiry" && time.Now().UnixMilli() < r.AccessExpiresAtMS {
					t.Fatal("selected TCP peer ended before original peer lease deadline")
				}

				transport.tcp.Close()
				select {
				case <-transport.tcp.exited:
				case <-time.After(time.Second):
					t.Fatal("stopped TCP counterpart did not join")
				}
				for id := range s.data.Nodes {
					if err := s.DeleteNode(id); err != nil {
						t.Fatal(err)
					}
				}
				outcome, _ := json.Marshal(map[string]any{"developmentInstalled": true, "networkTCPVerified": true, "networkStopped": true, "nativeSavedBeforeFailure": true, "inputAuthorized": false})
				if err := writeRegistrarFixtureAtomic(dir, "outcome.json", outcome); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		displayUnavailable := setupCase == "result-preexisting" || setupCase == "result-write-failure" || setupCase == "result-open-failure"
		if os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY") != "" && setupCase != "" && setupCase != "success" && !strings.HasPrefix(setupCase, "network-tcp") && !displayUnavailable {
			if _, err := os.Stat(filepath.Join(dir, "installed-cancel-request")); err == nil {
				_ = nativeCmd.Process.Signal(os.Interrupt)
			}
			if _, err := os.Stat(filepath.Join(dir, "installed-output-ended")); err == nil {
				err := nativeCmd.Wait()
				nativeWaited = true
				if err == nil {
					t.Fatal("expected stopped installed attempt succeeded")
				}
				snapshot, readErr := os.ReadFile(filepath.Join(dir, "installed-setup", "native", "state.json"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				saved := os.Getenv("MNET_DEVELOPMENT_SETUP_CASE") == "map-failure"
				var image map[string]any
				if json.Unmarshal(snapshot, &image) != nil || image["node_id"] != "" || image["node_token"] != "" {
					t.Fatal("stopped installed attempt changed legacy State shape")
				}
				if !saved && image["native_enrollment_v1"] != nil {
					t.Fatal("stopped installed attempt unexpectedly saved a credential")
				}
				if saved {
					encoded, ok := image["native_enrollment_v1"].(string)
					if !ok {
						t.Fatal("post-save failure lost its credential")
					}
					var outer struct {
						Document json.RawMessage `json:"document"`
					}
					if json.Unmarshal([]byte(encoded), &outer) != nil {
						t.Fatal("invalid native wrapper")
					}
					doc, err := contract.DecodeNativeCredentialDocument(outer.Document)
					if err != nil {
						t.Fatal("invalid retained credential")
					}
					s.mu.Lock()
					record := s.registrarNodeLocked(doc.Receipt.NodeID)
					matches := record != nil && record.ReceiptDigest == doc.ReceiptDigest && record.DevicePublicKey == doc.Receipt.DevicePublicKey
					s.mu.Unlock()
					if !matches || mapResponseRefusals.Load() != 1 {
						t.Fatal("saved credential did not match actual committed registrar node or map was retried")
					}
				}
				if os.Getenv("MNET_DEVELOPMENT_SETUP_CASE") == "launcher-hang" {
					journal, err := os.ReadFile(filepath.Join(dir, "installed-setup", "native", "enrollment-pending", "enrollment-pending-v1.json"))
					if err != nil || !bytes.Contains(journal, []byte(`"prepared"`)) {
						t.Fatal("launcher failure did not retain original prepared attempt")
					}
				}
				remoteCommitted := saved || os.Getenv("MNET_DEVELOPMENT_SETUP_CASE") == "lost-response"
				expectedNodes := 2
				if remoteCommitted {
					expectedNodes = 3
				}
				if len(s.data.Nodes) != expectedNodes {
					t.Fatal("stopped installed remote outcome differs", len(s.data.Nodes))
				}
				if remoteCommitted {
					journalPath := filepath.Join(dir, "installed-setup", "native", "enrollment-pending", "enrollment-pending-v1.json")
					journalBefore, err := os.ReadFile(journalPath)
					expectedPhase := "workflow-submit-may-have-started"
					if saved {
						expectedPhase = "native-finalization-completed"
					}
					if err != nil || !bytes.Contains(journalBefore, []byte(expectedPhase)) {
						t.Fatal("original pending outcome not retained", err)
					}
					repeat := exec.Command(os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY"), "--disposable-root", filepath.Join(dir, "installed-setup"))
					if repeat.Run() == nil {
						t.Fatal("uncertain installed attempt silently restarted")
					}
					journalAfter, err := os.ReadFile(journalPath)
					stateAfter, stateErr := os.ReadFile(filepath.Join(dir, "installed-setup", "native", "state.json"))
					if err != nil || stateErr != nil || !bytes.Equal(journalBefore, journalAfter) || !bytes.Equal(snapshot, stateAfter) || historyCalls.Load() != 0 || workflowSubmits.Load() != 2 || saved && mapResponseRefusals.Load() != 1 {
						t.Fatal("refused repeated caller changed uncertain owner or retried")
					}
				}
				result, readErr := os.ReadFile(filepath.Join(dir, "installed-setup", "setup-result.html"))
				if readErr != nil || bytes.Contains(result, []byte("<h1>Joining saved.")) {
					t.Fatal("stopped installed attempt claimed success", readErr)
				}
				for id := range s.data.Nodes {
					if err := s.DeleteNode(id); err != nil {
						t.Fatal(err)
					}
				}
				outcome, _ := json.Marshal(map[string]any{"developmentStopped": true, "noNativeCredential": !saved, "nativeSavedBeforeFailure": saved, "noRegistrarNode": !remoteCommitted, "remoteCommitted": remoteCommitted, "inputAuthorized": false})
				if err := writeRegistrarFixtureAtomic(dir, "outcome.json", outcome); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		if transport != nil {
			s.mu.Lock()
			var n *netmap.Node
			for _, candidate := range s.data.Nodes {
				if candidate.ID != r.NodeID && candidate.ID != peerRecord.NodeID {
					copy := *candidate
					copy.Endpoints = append([]string(nil), candidate.Endpoints...)
					n = &copy
					break
				}
			}
			s.mu.Unlock()
			_, nativeRelayReady := os.Stat(filepath.Join(dir, "native-relay-socket-ready"))
			if n != nil && (!relayMode.Load() || nativeRelayReady == nil) {
				transportNativeID = n.ID
				if !transport.revoked && !(setupCase == "network-tcp-expiry" && time.Now().UnixMilli() >= r.AccessExpiresAtMS) {
					firstMap, err := s.registrarCredentialMap(ref, secret)
					if err != nil {
						nativeCmd.Process.Kill()
						nativeCmd.Wait()
						nativeWaited = true

						t.Fatalf("transport current map %v received=%d sent=%v; native output: %s", err, transport.received, transport.sent, nativeOutput.String())
					}
					if relayMode.Load() {
						if _, err := os.Stat(filepath.Join(dir, "native-relay-reopened")); err == nil {
							transport.reopen = true
						}
					}
					transport.tick(t, firstMap, *n)
					if transport.reopenReceived > 0 {
						if err := writeRegistrarFixtureAtomic(dir, "peer-relay-reopened-received", []byte("fresh marked encrypted packet received after explicit native socket replacement")); err != nil {
							t.Fatal(err)
						}
					}
					if relayMode.Load() && transport.lastKey != "" {
						if err := writeRegistrarFixtureAtomic(dir, "transport-relay-peer-configured", []byte("exact enrolled native peer configured")); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := os.Stat(filepath.Join(dir, "native-transport-ready")); err == nil && transport.received > 0 && transport.sent && setupCase != "network-tcp-cancel" && setupCase != "network-tcp-expiry" {
						if err := s.DeleteNode(r.NodeID); err != nil {
							t.Fatal(err)
						}
						transport.revoked = true
						transportReceivedAtRevoke = transport.received
						// Discard only packets decrypted before the revocation
						// marker; native removes authorization before its next send.
						if transport.tcp != nil {
							transport.tcp.Close()
						} else {
							for {
								select {
								case <-transport.tun.Inbound:
								default:
									goto drainedTransport
								}
							}
						}
					drainedTransport:
						if err := os.WriteFile(filepath.Join(dir, "transport-peer-revoked"), []byte("actual Store delete committed"), 0600); err != nil {
							t.Fatal(err)
						}
					}
				} else if transport.tcp != nil {
					if setupCase == "network-tcp-expiry" {
						transport.tcp.Close()
					}
					if _, err := os.Stat(filepath.Join(dir, "native-tcp-revocation-checked")); err == nil {
						if !postRemovalVerified {
							select {
							case err := <-transport.tcp.done:
								if err == nil {
									t.Fatal("retired TCP host read unexpectedly succeeded")
								}
							case <-time.After(time.Second):
								t.Fatal("retired TCP host read did not join")
							}
							if err := writeRegistrarFixtureAtomic(dir, "transport-post-removal-verified", []byte("actual TCP peer scope closed and denied after Store removal")); err != nil {
								t.Fatal(err)
							}
							postRemovalVerified = true
						}
					}
				} else {
					if !postRemovalBarrier {
						if _, err := os.Stat(filepath.Join(dir, "native-transport-removal-ready")); err == nil {
							for {
								select {
								case <-transport.tun.Inbound:
								default:
									goto removalDrained
								}
							}
						removalDrained:
							if err := os.WriteFile(filepath.Join(dir, "transport-post-removal-barrier"), []byte("pre-removal packets drained"), 0600); err != nil {
								t.Fatal(err)
							}
							postRemovalBarrier = true
						}
					} else {
						if transport.reverseSent < 5 {
							transport.sendRevokedReverse(t, *n)
						}
						select {
						case packet := <-transport.tun.Inbound:
							original := tuntest.Ping(netip.MustParseAddr(transport.selfIP), netip.MustParseAddr(n.IPv4))
							reopened := append([]byte{}, original...)
							reopened[len(reopened)-1] ^= 0x31
							if !relayMode.Load() || !bytes.Equal(packet, original) && !bytes.Equal(packet, reopened) {
								t.Fatal("new encrypted payload decrypted after native removed revoked peer")
							}
						default:
						}
						if _, err := os.Stat(filepath.Join(dir, "native-transport-revocation-checked")); err == nil {
							if postRemovalCheckedAt.IsZero() {
								postRemovalCheckedAt = time.Now()
							}
							if !postRemovalVerified && transport.reverseSent == 5 && time.Since(postRemovalCheckedAt) >= 200*time.Millisecond {
								if err := os.WriteFile(filepath.Join(dir, "transport-post-removal-verified"), []byte("five new packets per direction denied after actual map removal"), 0600); err != nil {
									t.Fatal(err)
								}
								postRemovalVerified = true
							}
						}
					}
				}
			}
		}
		raw, err = os.ReadFile(filepath.Join(dir, "native-workflow-outcome.json"))
		if err == nil && setupCase != "network-tcp-expiry" {
			break
		}
		if time.Now().After(deadline) {
			nativeCmd.Process.Kill()
			nativeCmd.Wait()
			nativeWaited = true
			t.Fatalf("actual native process timeout: %s", nativeOutput.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := nativeCmd.Wait(); err != nil {
		nativeWaited = true
		t.Fatalf("actual native process failed: %v\n%s", err, nativeOutput.String())
	}
	nativeWaited = true
	var nativeOutcome struct {
		Receipt                   contract.WorkflowReceiptWire
		NativeOwnedKeyConfirmed   bool
		NativeProofStateUnchanged bool
		NativeStateFinalized      bool
		NativeCurrentPeers        int
		NativeHistoryRecovered    bool
		History                   contract.HistoryReceiptWire
		NativeEncryptedDirect     bool
		NativeEncryptedRelay      bool
	}
	if json.Unmarshal(raw, &nativeOutcome) != nil || !nativeOutcome.NativeProofStateUnchanged || (!recoveryMode.Load() && !preparedFinalize.Load() && (!nativeOutcome.NativeOwnedKeyConfirmed || !nativeOutcome.NativeStateFinalized || nativeOutcome.NativeCurrentPeers != 2 && os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY") == "")) || (preparedFinalize.Load() && (!nativeOutcome.NativeHistoryRecovered || nativeOutcome.NativeOwnedKeyConfirmed || !nativeOutcome.NativeStateFinalized || nativeOutcome.NativeCurrentPeers != 2 || (historyCalls.Load() != 1 && !preparedHistoryFinalize.Load()) || (preparedHistoryFinalize.Load() && historyCalls.Load() != 2))) || (recoveryMode.Load() && (!nativeOutcome.NativeHistoryRecovered || nativeOutcome.NativeOwnedKeyConfirmed || nativeOutcome.NativeStateFinalized != historyFinalize.Load() || historyCalls.Load() != 1 || (historyFinalize.Load() && nativeOutcome.NativeCurrentPeers != 2))) {
		t.Fatalf("missing actual daemon proof result owned=%v unchanged=%v finalized=%v peers=%d receipt=%v", nativeOutcome.NativeOwnedKeyConfirmed, nativeOutcome.NativeProofStateUnchanged, nativeOutcome.NativeStateFinalized, nativeOutcome.NativeCurrentPeers, nativeOutcome.Receipt.Receipt.NodeID != "")
	}

	if os.Getenv("MNET_DEVELOPMENT_SETUP_BINARY") != "" && !networkCase {
		setupCase := os.Getenv("MNET_DEVELOPMENT_SETUP_CASE")
		if setupCase == "result-preexisting" || setupCase == "result-write-failure" || setupCase == "result-open-failure" {
			root := filepath.Join(dir, "installed-setup")
			defer os.Chmod(root, 0700)
			if !strings.Contains(nativeOutput.String(), "Optional local result display unavailable") || strings.Contains(nativeOutput.String(), "Setup did not finish") {
				t.Fatal("optional display failure hid authoritative completion")
			}
			result, err := os.ReadFile(filepath.Join(root, "setup-result.html"))
			switch setupCase {
			case "result-preexisting":
				if err != nil || string(result) != "preserve-existing-result" {
					t.Fatal("preexisting result overwritten")
				}
			case "result-write-failure":
				if !os.IsNotExist(err) {
					t.Fatal("write-refused result unexpectedly created")
				}
			case "result-open-failure":
				if err != nil || !bytes.Contains(result, []byte("<h1>Joining saved.")) {
					t.Fatal("result opener failure lost saved page")
				}
			}
		}
		nodeID := nativeOutcome.Receipt.Receipt.NodeID
		record := s.registrarNodeLocked(nodeID)
		if record == nil || len(s.data.Nodes) != 3 {
			t.Fatal("installed source proof absent from actual Store")
		}
		installedRef := registrarCredentialReference{NodeID: record.NodeID, NetworkID: record.NetworkID, DevicePublicKey: record.DevicePublicKey, Scope: record.Scope, ReceiptID: record.ReceiptID, ReceiptDigest: record.ReceiptDigest, DescriptorDigest: record.DescriptorDigest, TranscriptDigest: record.Consumption.TranscriptDigest}
		current, err := s.registrarCredentialMap(installedRef, getPeerSecret())
		if err != nil || len(current.Peers) != nativeOutcome.NativeCurrentPeers {
			t.Fatal("installed native current map not source grounded", err)
		}
		ids := []string{}
		for id := range s.data.Nodes {
			ids = append(ids, id)
		}
		for _, id := range ids {
			if err := s.DeleteNode(id); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.registrarCredentialMap(installedRef, getPeerSecret()); err == nil {
			t.Fatal("deleted installed credential remained current")
		}
		outcome, _ := json.Marshal(map[string]any{"developmentInstalled": true, "nativeOwnedKeyConfirmed": true, "nativeProofStateUnchanged": true, "nativeStateFinalized": true, "nativeCurrentPeers": nativeOutcome.NativeCurrentPeers, "nodeDeleted": true, "inputAuthorized": false})
		if err := writeRegistrarFixtureAtomic(dir, "outcome.json", outcome); err != nil {
			t.Fatal(err)
		}
		return
	}
	if transportMode.Load() {
		if networkCase && !postRemovalVerified {
			if _, err := os.Stat(filepath.Join(dir, "native-tcp-revocation-checked")); err != nil {
				t.Fatal("installed TCP retirement evidence absent")
			}
			select {
			case err := <-transport.tcp.done:
				if err == nil {
					t.Fatal("retired installed TCP host unexpectedly succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("installed TCP host not joined")
			}
			postRemovalVerified = true
		}
		if transport == nil || !transport.revoked || transportReceivedAtRevoke == 0 || transport.tcp == nil && transport.reverseSent != 5 || !(nativeOutcome.NativeEncryptedDirect || nativeOutcome.NativeEncryptedRelay) || !postRemovalVerified || transportNativeID == "" || len(s.data.Nodes) != 2 {
			t.Fatal("connected encrypted direct transport/revocation evidence absent")
		}
		if _, err := s.registrarCredentialMap(ref, secret); err == nil {
			t.Fatal("revoked transport credential retained current map")
		}
		nativeRecord := s.registrarNodeLocked(transportNativeID)
		nativeRef := registrarCredentialReference{NodeID: nativeRecord.NodeID, NetworkID: nativeRecord.NetworkID, DevicePublicKey: nativeRecord.DevicePublicKey, Scope: nativeRecord.Scope, ReceiptID: nativeRecord.ReceiptID, ReceiptDigest: nativeRecord.ReceiptDigest, DescriptorDigest: nativeRecord.DescriptorDigest, TranscriptDigest: nativeRecord.Consumption.TranscriptDigest}
		nativeMap, err := s.registrarCredentialMap(nativeRef, getPeerSecret())
		leaseEnded := os.Getenv("MNET_DEVELOPMENT_NETWORK_SESSION") == "1" && os.Getenv("MNET_DEVELOPMENT_NETWORK_SESSION_CASE") == "expiry"
		if leaseEnded {
			if err == nil || time.Now().UnixMilli() < nativeOutcome.Receipt.Receipt.AccessExpiresAtMS {
				t.Fatal("expired disposable session regained current map")
			}
		} else if err != nil || len(nativeMap.Peers) != 1 {
			t.Fatal("native current map retained revoked transport peer", err)
		}
		if err := s.DeleteNode(nativeRecord.NodeID); err != nil {
			t.Fatal(err)
		}
	} else if recoveryMode.Load() && !historyFinalize.Load() {
		if len(s.data.Nodes) != 2 || nativeOutcome.History.Receipt.NodeID == "" || time.Now().UnixMilli() < nativeOutcome.History.Receipt.AccessExpiresAtMS {
			t.Fatal("expired deleted native history not recovered")
		}
		if _, err := s.registrarCredentialMap(ref, secret); err == nil {
			t.Fatal("expired current access revived by history")
		}
	} else {
		nativeNodeID := nativeOutcome.Receipt.Receipt.NodeID
		if historyFinalize.Load() || preparedFinalize.Load() {
			nativeNodeID = nativeOutcome.History.Receipt.NodeID
		}
		nativeRecord := s.registrarNodeLocked(nativeNodeID)
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

	}
	bridgeMu.Lock()
	peerSecret = peerSavedSecret
	bridgeMu.Unlock()
	if !transportMode.Load() {
		if err := s.DeleteNode(r.NodeID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.registrarCredentialMap(ref, secret); err == nil {
		t.Fatal("deleted node still usable")
	}
	if got, err := s.registrarHistoricalReceipt(c, secret); err != nil || got != history {
		t.Fatal("delete erased history", err)
	}
	peerMap, err = s.registrarCredentialMap(peerRef, getPeerSecret())
	if recoveryMode.Load() && !historyFinalize.Load() || os.Getenv("MNET_DEVELOPMENT_NETWORK_SESSION") == "1" && os.Getenv("MNET_DEVELOPMENT_NETWORK_SESSION_CASE") == "expiry" {
		if err == nil {
			t.Fatal("history/session revived expired second credential")
		}
	} else if err != nil || len(peerMap.Peers) != 0 {
		t.Fatal("deleted peer remained in second map", err)
	}
	if err := s.DeleteNode(peerRecord.NodeID); err != nil {
		t.Fatal(err)
	}
	if networkCase {
		out, _ := json.Marshal(map[string]any{"developmentInstalled": true, "nativeOwnedKeyConfirmed": true, "nativeProofStateUnchanged": true, "nativeStateFinalized": true, "nativeCurrentPeers": nativeOutcome.NativeCurrentPeers, "nodeDeleted": true, "inputAuthorized": false, "networkTCPVerified": true, "networkStopped": true})
		if err := writeRegistrarFixtureAtomic(dir, "outcome.json", out); err != nil {
			t.Fatal(err)
		}
		return
	}
	out, _ := json.Marshal(map[string]any{"sourceDenied": false, "bidirectionalActualPeers": true, "nativeOwnedKeyConfirmed": !recoveryMode.Load() && !preparedFinalize.Load(), "threeActualPeers": true, "nativeProofStateUnchanged": true, "nativeStateFinalized": !recoveryMode.Load() || historyFinalize.Load(), "nativeCurrentPeers": nativeOutcome.NativeCurrentPeers, "nativeHistoryRecovered": recoveryMode.Load() || preparedFinalize.Load(), "nativeEncryptedDirect": transportMode.Load() && !relayMode.Load(), "nativeEncryptedRelay": relayMode.Load(), "journalWorkflowReceiptPinned": true, "nodeCreated": true, "nodeDeleted": true, "currentMapBeforeDelete": true, "legacyTokenCreated": false, "inputAuthorized": false, "receiptId": r.ReceiptID, "receiptDigest": r.ReceiptDigest, "reserveId": req.ReserveID, "descriptorDigest": descriptor, "handoffId": own.ID})
	if err := writeRegistrarFixtureAtomic(dir, "outcome.json", out); err != nil {
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

func writeRegistrarFixtureAtomic(dir, name string, raw []byte) error {
	path := filepath.Join(dir, name)
	temp := path + ".tmp"
	if err := os.WriteFile(temp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func newInstalledDevelopmentFixtureCmd(t *testing.T, dir, installed string, policy config.LocalConfigV2, limits registrar.WorkflowLimits) *exec.Cmd {
	t.Helper()

	root := filepath.Join(dir, "installed-setup")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "DISPOSABLE-MNET-SETUP"), []byte("mnet-disposable-enrollment-v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	policyRaw, _ := json.Marshal(policy)
	limitRaw, _ := json.Marshal(limits)
	if err := os.WriteFile(filepath.Join(root, "enrollment-policy-v2.json"), policyRaw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "workflow-limits.json"), limitRaw, 0600); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("MNET_DEVELOPMENT_SETUP_CASE") == "result-preexisting" {
		if err := os.WriteFile(filepath.Join(root, "setup-result.html"), []byte("preserve-existing-result"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	nativeCmd := exec.Command(installed, "--disposable-root", root)
	if strings.HasPrefix(os.Getenv("MNET_DEVELOPMENT_SETUP_CASE"), "network-tcp") {
		nativeCmd.Args = append(nativeCmd.Args, "--disposable-network-tcp")
	}
	if os.Getenv("MNET_DEVELOPMENT_SETUP_BROWSER") == "1" {
		nativeCmd.Args = append(nativeCmd.Args, "--open-browser")
	}
	pipe, err := nativeCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer func() { _ = writeRegistrarFixtureAtomic(dir, "installed-output-ended", []byte("ended")) }()
		var savedResult json.RawMessage
		verifiedTCP := false
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(make([]byte, 4096), 32768)
		for scanner.Scan() {
			var event struct {
				Stage  string                 `json:"stage"`
				Launch *contract.SourceLaunch `json:"launch"`
				Result json.RawMessage        `json:"result"`
			}
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				continue
			}
			if event.Launch != nil {
				raw, _ := json.Marshal(event.Launch)
				if err := writeRegistrarFixtureAtomic(dir, "native-launch.json", raw); err != nil {
					t.Error(err)
				}
			}
			networkCase := strings.HasPrefix(os.Getenv("MNET_DEVELOPMENT_SETUP_CASE"), "network-tcp")
			if event.Stage == "joined-not-connected" && len(event.Result) > 0 {
				savedResult = append(json.RawMessage(nil), event.Result...)
				if !networkCase {
					if err := writeRegistrarFixtureAtomic(dir, "native-workflow-outcome.json", savedResult); err != nil {
						t.Error(err)
					}
				}
			}
			if networkCase && event.Stage == "development-network-verified" {
				verifiedTCP = true
				if err := writeRegistrarFixtureAtomic(dir, "native-transport-ready", []byte("actual installed userspace TCP response verified")); err != nil {
					t.Error(err)
				}
			}
			if networkCase && event.Stage == "development-network-finished" && verifiedTCP && len(savedResult) > 0 {
				var result map[string]any
				if json.Unmarshal(savedResult, &result) != nil {
					t.Error("invalid saved result")
					continue
				}
				result["NativeEncryptedDirect"] = true
				raw, _ := json.Marshal(result)
				if err := writeRegistrarFixtureAtomic(dir, "native-tcp-revocation-checked", []byte("actual installed peer scope retired and owner joined")); err != nil {
					t.Error(err)
				}
				if err := writeRegistrarFixtureAtomic(dir, "native-workflow-outcome.json", raw); err != nil {
					t.Error(err)
				}
			}
		}
	}()
	return nativeCmd
}
