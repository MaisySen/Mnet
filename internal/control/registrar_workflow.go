package control

// Private, unregistered DEVELOPMENT enrollment owner. It composes actual
// configured source transport, owned proof and one actual Store publication.
// Explicit registrar policy issues only a bounded initial development lease;
// source reserve is consent/eligibility evidence, NOT a source-issued grant.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
	"fintex.au/mnet/internal/netmap"
)

const registrarEntryByteReserve = 256 * 1024

var errRegistrarEnrollment = errors.New("development enrollment refused; reconcile same attempt before any new authority")

type registrarEnrollmentOwner struct {
	revision [32]byte
	sources  *registrarSourceOwner
	config   registrarWorkflowConfig
	policy   *config.LocalPolicyV2
	attempts map[string]*registrarEnrollmentAttempt
	live     int
	retired  bool
}
type registrarEnrollmentAttempt struct {
	proofAttempts int
	requestDigest [32]byte
	started       time.Time
	challenge     contract.Challenge
	proof         *proof.Server
	state         string
	response      [32]byte
	confirmation  [32]byte
}
type registrarEnrollmentResult struct {
	History      registrarReceiptHistory
	Confirmation [32]byte
}

func (registrarEnrollmentResult) String() string {
	return "[private development enrollment historical result]"
}
func (registrarEnrollmentResult) GoString() string {
	return "[private development enrollment historical result]"
}
func (o *registrarEnrollmentOwner) close() {
	o.retired = true
	for _, a := range o.attempts {
		if a.proof != nil {
			a.proof.Close()
		}
	}
}
func (o *registrarEnrollmentOwner) expire(now time.Time) {
	for _, a := range o.attempts {
		if a.state == "challenge" && now.UnixMilli() >= a.challenge.ExpiresAtMS {
			a.proof.Close()
			a.state = "expired"
			o.live--
		}
	}
}
func registrarBootstrapMatches(secret, digest string) bool {
	if !registrarCredentialSecret(secret) {
		return false
	}
	want, err := base64.RawURLEncoding.Strict().DecodeString(digest)
	got := sha256.Sum256([]byte(secret))
	return err == nil && len(want) == 32 && subtle.ConstantTimeCompare(got[:], want) == 1
}
func (s *Store) registrarAdmissionLocked(request contract.SourceReserveRequest, o *registrarEnrollmentOwner) error {
	if s.validateRegistrarState() != nil || s.data.Networks[request.Mapping.NetworkID] == nil {
		return errRegistrarEnrollment
	}
	rawKey, err := base64.RawURLEncoding.Strict().DecodeString(request.Launch.DevicePublicKey)
	if err != nil || len(rawKey) != 32 {
		return errRegistrarEnrollment
	}
	key := base64.StdEncoding.EncodeToString(rawKey)
	for _, n := range s.data.Nodes {
		if n.NetworkID == request.Mapping.NetworkID && n.PubKey == key {
			return errRegistrarEnrollment
		}
	}
	count := 0
	if s.data.Registrar != nil {
		count = len(s.data.Registrar.Records)
		for _, r := range s.data.Registrar.Records {
			if r.NetworkID == request.Mapping.NetworkID && r.DevicePublicKey == key {
				return errRegistrarEnrollment
			}
			if r.Scope.Issuer == request.Mapping.Issuer && r.Scope.RegistrarOrigin == request.Mapping.RegistrarOrigin && (r.Scope.HandoffID == request.HandoffID || r.Scope.AttemptID == request.Launch.AttemptID || r.Scope.ReserveID == request.ReserveID) {
				return errRegistrarEnrollment
			}
		}
	}
	if count >= o.config.MaxRecords {
		return errRegistrarEnrollment
	}
	// Reserve a conservative whole-entry envelope BEFORE IDs/IP/DH/source I/O.
	// Explicit serialized-byte capacity, no unbounded history/eviction promise.
	image, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil || len(image) > o.config.MaxStateBytes-registrarEntryByteReserve {
		return errRegistrarEnrollment
	}
	return nil
}

// beginRegistrarEnrollment is the only challenge producer. Caller may propose
// original private ticket/launch/mapping, never a Binding/clock/evidence/permit.
// Installed private files select source/service/principals, policy and limits.
func (s *Store) beginRegistrarEnrollment(ctx context.Context, request contract.SourceReserveRequest, secret string) (contract.Challenge, error) {
	var empty contract.Challenge
	if ctx == nil || ctx.Err() != nil || !registrarBootstrapMatches(secret, request.Launch.CredentialDigest) {
		return empty, errRegistrarEnrollment
	}
	s.mu.Lock()
	o, err := s.registrarWorkflowLocked()
	if err != nil {
		s.mu.Unlock()
		return empty, err
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > 16384 {
		s.mu.Unlock()
		return empty, errRegistrarEnrollment
	}
	requestDigest := sha256.Sum256(raw)
	if _, err = contract.ServiceReserveDescriptorBytes(request, o.config.Policy.ServiceID); err != nil {
		s.mu.Unlock()
		return empty, errRegistrarEnrollment
	}
	mappingAllowed := false
	for _, m := range o.policy.SourceMappings() {
		if m == request.Mapping {
			mappingAllowed = true
		}
	}
	if !mappingAllowed {
		s.mu.Unlock()
		return empty, errRegistrarEnrollment
	}
	if a := o.attempts[request.Launch.AttemptID]; a != nil {
		if a.requestDigest == requestDigest && a.state == "challenge" && s.registrarAttemptCurrentLocked(o, a) == nil {
			c := a.challenge
			s.mu.Unlock()
			return c, nil
		}
		s.mu.Unlock()
		return empty, errRegistrarEnrollment
	}
	o.expire(time.Now())
	records := 0
	if s.data.Registrar != nil {
		records = len(s.data.Registrar.Records)
	}
	if len(o.attempts) >= o.config.MaxAttempts || o.live >= o.config.MaxLive || records+o.live >= o.config.MaxRecords || s.registrarAdmissionLocked(request, o) != nil {
		s.mu.Unlock()
		return empty, errRegistrarEnrollment
	}
	image, _ := json.MarshalIndent(&s.data, "", "  ")
	if len(image)+(o.live+1)*registrarEntryByteReserve > o.config.MaxStateBytes {
		s.mu.Unlock()
		return empty, errRegistrarEnrollment
	}
	a := &registrarEnrollmentAttempt{requestDigest: requestDigest, started: time.Now(), state: "source-may-have-started"}
	o.attempts[request.Launch.AttemptID] = a
	o.live++
	s.mu.Unlock()
	observed, sourceErr := s.reserveRegistrarSource(ctx, o.config.SourceID, request)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, currentErr := s.registrarWorkflowLocked()
	if sourceErr != nil || currentErr != nil || current != o || ctx.Err() != nil {
		a.state = "source-unknown"
		o.live--
		return empty, errRegistrarEnrollment
	}
	p := observed.Provenance
	o.expire(time.Now())
	principalLive := 0
	for _, other := range o.attempts {
		b := other.challenge.Binding
		if other.state == "challenge" && b.PrincipalAuthority == p.PrincipalAuthority && b.PrincipalNamespace == p.PrincipalNamespace && b.PrincipalID == p.PrincipalID {
			principalLive++
		}
	}
	if principalLive >= o.config.MaxLivePerPrincipal {
		a.state = "principal-capacity"
		o.live--
		return empty, errRegistrarEnrollment
	}
	if !o.policy.AllowsObservedDATA(observed.Mapping, p.PrincipalAuthority, p.PrincipalNamespace, p.PrincipalID, observed.ServiceID) || s.registrarAdmissionLocked(request, o) != nil {
		a.state = "source-refused"
		o.live--
		return empty, errRegistrarEnrollment
	}
	checked, err := time.Parse(time.RFC3339Nano, observed.CheckedAt)
	if err != nil {
		a.state = "source-refused"
		o.live--
		return empty, errRegistrarEnrollment
	}
	deadline, err := time.Parse(time.RFC3339Nano, observed.Deadline)
	if err != nil {
		a.state = "source-refused"
		o.live--
		return empty, errRegistrarEnrollment
	}
	// Policy is registrar-issued development duration, NOT a Multies access
	// grant. No renewal or implicit production duration is implemented.
	access := checked.Add(time.Duration(o.config.MaxLeaseMS) * time.Millisecond)
	// For this INITIAL development lease only, its usable end cannot outlive
	// either proof freshness or observation freshness. Thus a rename paused past
	// those bounds cannot leave a newly usable late network grant on reopen.
	for _, end := range []time.Time{deadline, checked.Add(time.Duration(o.config.ChallengeMS) * time.Millisecond), checked.Add(time.Duration(o.config.MaxObservationAgeMS) * time.Millisecond)} {
		if end.Before(access) {
			access = end
		}
	}
	expires := access
	b := contract.Binding{PrincipalAuthority: p.PrincipalAuthority, PrincipalNamespace: p.PrincipalNamespace, PrincipalID: p.PrincipalID, SessionBinding: p.SessionBinding, IssuerLabel: observed.Mapping.Issuer, RegistrarOrigin: observed.Mapping.RegistrarOrigin, Audience: observed.Mapping.Audience, ServiceID: observed.ServiceID, WorkspaceID: observed.Mapping.WorkspaceID, NetworkID: observed.Mapping.NetworkID, MappingRevision: observed.Mapping.MappingRevision, HandoffID: observed.HandoffID, AttemptID: observed.Launch.AttemptID, ReserveID: observed.ReserveID, DescriptorDigest: observed.DescriptorDigest, DescriptorEncodingVersion: observed.DescriptorEncodingVersion, DevicePublicKey: observed.Launch.DevicePublicKey, CredentialDigest: observed.Launch.CredentialDigest, CheckedAtMS: checked.UnixMilli(), ReserveDeadlineMS: deadline.UnixMilli(), AccessUntilMS: access.UnixMilli()}
	// Validate source age BEFORE generating any DH/ID work. Probe is DATA only.
	probe := contract.Challenge{Binding: b, ID: b.AttemptID, Nonce: b.SessionBinding, EphemeralPublicKey: b.DevicePublicKey, ExpiresAtMS: expires.UnixMilli()}
	if contract.CheckFixtureTime(probe, time.Now().UnixMilli(), o.timing()) != nil {
		a.state = "source-expired"
		o.live--
		return empty, errRegistrarEnrollment
	}
	for tries := 0; tries < 4; tries++ {
		server, c, err := proof.NewServer(b, time.Now(), expires, time.Duration(o.config.ChallengeMS)*time.Millisecond)
		if err != nil {
			break
		}
		collision := c.ID == c.Nonce
		for _, other := range o.attempts {
			if other.challenge.ID == c.ID || other.challenge.Nonce == c.Nonce || other.challenge.ID == c.Nonce || other.challenge.Nonce == c.ID {
				collision = true
			}
		}
		if s.data.Registrar != nil {
			if _, exists := s.data.Registrar.ConsumedProofs[registrarProofKey(c)]; exists {
				collision = true
			}
			for _, r := range s.data.Registrar.Records {
				if r.Consumption != nil && (r.Consumption.Challenge.ID == c.ID || r.Consumption.Challenge.Nonce == c.Nonce || r.Consumption.Challenge.ID == c.Nonce || r.Consumption.Challenge.Nonce == c.ID) {
					collision = true
				}
			}
		}
		if collision {
			server.Close()
			continue
		}
		a.proof = server
		a.challenge = c
		a.state = "challenge"
		if s.registrarAttemptCurrentLocked(o, a) != nil {
			server.Close()
			a.state = "challenge-refused"
			o.live--
			return empty, errRegistrarEnrollment
		}
		return c, nil
	}
	a.state = "challenge-refused"
	o.live--
	return empty, errRegistrarEnrollment
}
func (o *registrarEnrollmentOwner) timing() contract.FixtureTiming {
	return contract.FixtureTiming{MaxObservationAgeMS: int64(o.config.MaxObservationAgeMS), MaxLeaseMS: int64(o.config.MaxLeaseMS), ClockUncertaintyMS: int64(o.config.ClockUncertaintyMS)}
}
func (s *Store) registrarAttemptCurrentLocked(o *registrarEnrollmentOwner, a *registrarEnrollmentAttempt) error {
	current, err := s.registrarWorkflowLocked()
	if err != nil || current != o || o.retired || a.state != "challenge" {
		return errRegistrarEnrollment
	}
	now := time.Now()
	if now.UnixMilli() < a.started.UnixMilli() {
		return errRegistrarEnrollment
	}
	elapsed := a.started.Add(time.Since(a.started))
	if now.Before(elapsed) {
		now = elapsed
	}
	return contract.CheckFixtureTime(a.challenge, now.UnixMilli(), o.timing())
}

// submit selects ONLY the server-owned original attempt, checks echoed ID and
// secret, then verifies side-effect-free proof before ONE Store publication.
// No caller-supplied challenge/binding/evidence/expiry can become a permit.
func (s *Store) submitRegistrarEnrollment(ctx context.Context, attemptID, challengeID, secret string, response [32]byte) (registrarEnrollmentResult, error) {
	var empty registrarEnrollmentResult
	if ctx == nil || ctx.Err() != nil {
		return empty, errRegistrarEnrollment
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := s.registrarWorkflowLocked()
	if err != nil {
		return empty, err
	}
	a := o.attempts[attemptID]
	if a == nil || a.challenge.ID != challengeID || !registrarBootstrapMatches(secret, a.challenge.Binding.CredentialDigest) {
		return empty, errRegistrarEnrollment
	}
	if a.state == "committed" {
		if subtle.ConstantTimeCompare(response[:], a.response[:]) != 1 {
			return empty, errRegistrarEnrollment
		}
		history, err := s.registrarHistoricalReceiptLocked(a.challenge, secret)
		if err != nil {
			return empty, err
		}
		return registrarEnrollmentResult{History: history, Confirmation: a.confirmation}, nil
	}
	if s.registrarAttemptCurrentLocked(o, a) != nil {
		return empty, errRegistrarEnrollment
	}
	if a.proofAttempts >= o.config.MaxProofAttempts {
		return empty, errRegistrarEnrollment
	}
	a.proofAttempts++
	confirmation, err := a.proof.Verify(challengeID, response, time.Now())
	if err != nil {
		return empty, errRegistrarEnrollment
	}
	b := a.challenge.Binding
	request := contract.SourceReserveRequest{HandoffID: b.HandoffID, ReserveID: b.ReserveID, Launch: contract.SourceLaunch{DevicePublicKey: b.DevicePublicKey, AttemptID: b.AttemptID}, Mapping: contract.SourceMapping{NetworkID: b.NetworkID, Issuer: b.IssuerLabel, RegistrarOrigin: b.RegistrarOrigin}}
	if s.registrarAdmissionLocked(request, o) != nil {
		return empty, errRegistrarEnrollment
	}
	// Reserve/claim/proof uniqueness was checked before identifiers and cursor.
	var idBytes [32]byte
	if _, err = rand.Read(idBytes[:]); err != nil || idBytes == [32]byte{} {
		return empty, errRegistrarEnrollment
	}
	receiptID := base64.RawURLEncoding.EncodeToString(idBytes[:])
	var nodeBytes [16]byte
	if _, err = rand.Read(nodeBytes[:]); err != nil || nodeBytes == [16]byte{} {
		return empty, errRegistrarEnrollment
	}
	nodeID := "registrar-" + base64.RawURLEncoding.EncodeToString(nodeBytes[:])
	if s.data.Nodes[nodeID] != nil || s.registrarNodeLocked(nodeID) != nil {
		return empty, errRegistrarEnrollment
	}
	oldCursor, hadCursor := s.data.NextIP[b.NetworkID]
	ip, err := s.allocIPLocked(b.NetworkID)
	if err != nil {
		return empty, err
	}
	restoreCursor := func() {
		if hadCursor {
			s.data.NextIP[b.NetworkID] = oldCursor
		} else {
			delete(s.data.NextIP, b.NetworkID)
		}
	}
	now := time.Now().UTC()
	expiry := time.UnixMilli(b.AccessUntilMS).UTC()
	key, _ := base64.RawURLEncoding.Strict().DecodeString(b.DevicePublicKey)
	node := &netmap.Node{ID: nodeID, NetworkID: b.NetworkID, PubKey: base64.StdEncoding.EncodeToString(key), Name: s.uniqueNodeNameLocked(b.NetworkID, "enrolled-"+b.AttemptID[:8]), IPv4: ip, Approved: true, CreatedAt: now, LastSeen: now, KeyExpiry: &expiry}
	receipt, err := contract.ReceiptV2ForChallenge(a.challenge, nodeID, receiptID, now.UnixMilli())
	if err != nil {
		restoreCursor()
		return empty, err
	}
	r := &registrarRecord{NetworkID: b.NetworkID, DevicePublicKey: node.PubKey, NodeID: nodeID, CredentialDigest: b.CredentialDigest, Phase: "active", Scope: registrarScope{Issuer: b.IssuerLabel, RegistrarOrigin: b.RegistrarOrigin, Audience: b.Audience, ServiceID: b.ServiceID, WorkspaceID: b.WorkspaceID, MappingRevision: b.MappingRevision, HandoffID: b.HandoffID, AttemptID: b.AttemptID, ReserveID: b.ReserveID}, DescriptorDigest: b.DescriptorDigest, DescriptorEncodingVersion: 1, ReceiptEncodingVersion: 2, ReceiptID: receiptID, ReceiptDigest: contract.ReceiptV2Digest(receipt), CommittedAtMS: receipt.CommittedAtMS, AccessExpiresAtMS: b.AccessUntilMS, Consumption: &registrarConsumption{Challenge: a.challenge, TranscriptDigest: registrarTranscriptDigest(a.challenge)}}
	oldRegistrar := s.data.Registrar
	candidate := &registrarState{Version: 2, Records: map[string]*registrarRecord{}, ConsumedProofs: map[string]string{}}
	if oldRegistrar != nil {
		for k, v := range oldRegistrar.Records {
			candidate.Records[k] = v
		}
		for k, v := range oldRegistrar.ConsumedProofs {
			candidate.ConsumedProofs[k] = v
		}
	}
	index := registrarClaimKey(r.NetworkID, r.DevicePublicKey)
	candidate.Records[index] = r
	candidate.ConsumedProofs[registrarProofKey(a.challenge)] = index
	s.data.Nodes[nodeID] = node
	s.data.Registrar = candidate
	rollback := func() { delete(s.data.Nodes, nodeID); s.data.Registrar = oldRegistrar; restoreCursor() }
	image, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil || len(image) > o.config.MaxStateBytes {
		rollback()
		return empty, errRegistrarEnrollment
	}
	guard := func() error {
		if ctx.Err() != nil {
			return errRegistrarEnrollment
		}
		return s.registrarAttemptCurrentLocked(o, a)
	}
	published, err := s.publishGuardedLocked(guard)
	if err != nil {
		if published {
			s.freezeLocked(errors.Join(ErrStoreUncertain, err))
			return empty, s.fault
		}
		rollback()
		return empty, err
	}
	a.state = "committed"
	a.response = response
	a.confirmation = confirmation
	a.proof.Close()
	o.live--
	s.relayIndex = nil
	s.bumpLocked(b.NetworkID)
	return registrarEnrollmentResult{History: registrarReceiptHistory{Receipt: receipt, Digest: r.ReceiptDigest}, Confirmation: confirmation}, nil
}
