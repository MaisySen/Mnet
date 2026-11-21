//go:build mnet_enrollment_development || multies_native_fixture

package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/privatefs"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const developmentV2Header = "MNET-DISPOSABLE-REGISTRAR-ONLY-STORE-V2\n"
const developmentV2Marker = "MNET-DISPOSABLE-REGISTRAR-ONLY-ROOT-V2\n"
const developmentLedgerReserve = 36864
const developmentLedgerEntryCap = 4096
const developmentV2ImageCap = 1048576

// Exact-source-byte fingerprint is a high-entropy-secret verifier, NOT secrecy.
// It leaks equality and enables guessing weak secrets, never log/disclose it.
func admissionDigest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func admissionConfigFingerprint(sources, workflow []byte) string {
	return admissionDigest([]any{"mnet-admission-config-v2", string(sources), string(workflow), 8, 4, developmentLedgerReserve, developmentV2ImageCap})
}

type developmentV2Codec struct{}

func (developmentV2Codec) maxBytes() int { return developmentV2ImageCap }
func (developmentV2Codec) decode(raw []byte) ([]byte, error) {
	if len(raw) > developmentV2ImageCap || !bytes.HasPrefix(raw, []byte(developmentV2Header)) {
		return nil, errRegistrarEnrollment
	}
	payload := raw[len(developmentV2Header):]
	if len(payload) == 0 || payload[0] != '{' {
		return nil, errRegistrarEnrollment
	}
	if !admissionV2PayloadExact(payload) {
		return nil, errRegistrarEnrollment
	}
	return payload, nil
}
func (developmentV2Codec) encode(payload []byte) ([]byte, error) {
	if len(payload) == 0 || payload[0] != '{' || len(payload) > developmentV2ImageCap-len(developmentV2Header) {
		return nil, errRegistrarEnrollment
	}
	return append([]byte(developmentV2Header), payload...), nil
}

type developmentAdmissionV2 struct{ s *Store }

func NewDisposableRegistrarV2(parent string) (*DisposableRegistrar, error) {
	root, e := os.MkdirTemp(parent, "mnet-disposable-admission-v2-")
	if e != nil {
		return nil, e
	}
	s, e := Open(filepath.Join(root, developmentStoreName))
	if e != nil {
		return nil, e
	}
	s.storageCodec = developmentV2Codec{}
	n, e := s.CreateNetwork("disposable-admission-v2")
	if e != nil {
		s.Close()
		return nil, e
	}
	if e = privatefs.WriteFile(filepath.Join(root, "root-kind"), []byte(developmentV2Marker)); e == nil {
		e = syncStoreDir(root)
	}
	if e != nil {
		s.Close()
		return nil, e
	}
	s.admissionPolicy = &developmentAdmissionV2{s}
	return &DisposableRegistrar{s, root, n.ID}, nil
}
func OpenDisposableRegistrarV2(root string) (*DisposableRegistrar, error) {
	info, e := os.Lstat(root)
	if e != nil || !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&os.ModeSymlink != 0 {
		return nil, errRegistrarEnrollment
	}
	marker := filepath.Join(root, "root-kind")
	info, e = os.Lstat(marker)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != int64(len(developmentV2Marker)) || singleLink(marker, info) != nil {
		return nil, errRegistrarEnrollment
	}
	raw, e := os.ReadFile(marker)
	if e != nil || !bytes.Equal(raw, []byte(developmentV2Marker)) {
		return nil, errRegistrarEnrollment
	}
	s, e := openStore(filepath.Join(root, developmentStoreName), developmentV2Codec{})
	if e != nil {
		return nil, e
	}
	network, e := developmentRegistrarOnly(s)
	if e != nil {
		s.Close()
		return nil, e
	}
	p := &developmentAdmissionV2{s}
	s.admissionPolicy = p
	if s.data.Admission != nil {
		if e = p.checkConfig(); e != nil {
			s.Close()
			return nil, e
		}
	}
	return &DisposableRegistrar{s, root, network}, nil
}
func (p *developmentAdmissionV2) checkConfig() error {
	l := p.s.data.Admission
	if l == nil {
		return errRegistrarEnrollment
	}
	sources, e := readRegistrarSources(p.s.path + ".registrar-sources-v1.json")
	if e != nil {
		return e
	}
	workflow, e := readRegistrarSources(p.s.path + ".registrar-workflow-v1.json")
	if e != nil {
		return e
	}
	if l.ConfigFingerprint != admissionConfigFingerprint(sources, workflow) {
		return errRegistrarEnrollment
	}
	var c registrarSourceConfig
	if json.Unmarshal(sources, &c) != nil || len(c.Sources) != 1 || l.SourceAuthorityDigest != admissionDigest([]string{c.Sources[0].SourceOrigin, c.Sources[0].PrincipalAuthority}) {
		return errRegistrarEnrollment
	}
	return p.validate()
}
func (p *developmentAdmissionV2) validate() error {
	l := p.s.data.Admission
	if l == nil || l.Version != 2 || l.SourceTotal < 0 || l.SourceTotal > 8 || len(l.Entries) != l.SourceTotal || len(l.PrincipalTotals) > 8 || !registrarCredentialSecret(l.ConfigFingerprint) || !registrarCredentialSecret(l.SourceAuthorityDigest) {
		return errRegistrarEnrollment
	}
	sources, err := readRegistrarSources(p.s.path + ".registrar-sources-v1.json")
	var source registrarSourceConfig
	if err != nil || json.Unmarshal(sources, &source) != nil || len(source.Sources) != 1 || l.SourceAuthorityDigest != admissionDigest([]string{source.Sources[0].SourceOrigin, source.Sources[0].PrincipalAuthority}) {
		return errRegistrarEnrollment
	}
	workflow, err := readRegistrarSources(p.s.path + ".registrar-workflow-v1.json")
	var config registrarWorkflowConfig
	if err != nil || json.Unmarshal(workflow, &config) != nil || l.ConfigFingerprint != admissionConfigFingerprint(sources, workflow) {
		return errRegistrarEnrollment
	}
	cap := config.MaxStateBytes
	if cap <= 0 || cap > developmentV2ImageCap {
		return errRegistrarEnrollment
	}
	image, err := json.MarshalIndent(&p.s.data, "", "  ")
	if err != nil || len(image) > cap-len(developmentV2Header) {
		return errRegistrarEnrollment
	}
	totals := map[string]int{}
	committed := map[string]bool{}
	for key, e := range l.Entries {
		if e == nil || key != e.AttemptID || !registrarCredentialSecret(key) || e.Version != 2 || e.Purpose != "mnet-initial-enrollment-v2" || e.SourceDebit != 1 || e.ConfigFingerprint != l.ConfigFingerprint || e.SourceAuthorityDigest != l.SourceAuthorityDigest || !registrarCredentialSecret(e.DescriptorDigest) || !registrarCredentialSecret(e.RequestDigest) || !registrarCredentialSecret(e.BootstrapDigest) {
			return errRegistrarEnrollment
		}
		switch e.Phase {
		case "source-work-reserved":
			if e.PrincipalDebit != 0 || e.PrincipalID != "" || e.PrincipalNamespace != "" || e.PrincipalTupleDigest != "" || e.ObservationDigest != "" || e.CheckedAtMS != 0 || e.ReserveDeadlineMS != 0 {
				return errRegistrarEnrollment
			}
		case "source-observed-principal-debited", "challenge-issued", "committed":
			if e.PrincipalDebit != 1 || !validAdmissionPrincipal(e.PrincipalID) || e.PrincipalNamespace != contract.PrincipalNamespace || e.PrincipalTupleDigest != admissionDigest([]string{source.Sources[0].PrincipalAuthority, e.PrincipalNamespace, e.PrincipalID}) || !registrarCredentialSecret(e.ObservationDigest) || e.CheckedAtMS <= 0 || e.ReserveDeadlineMS <= e.CheckedAtMS {
				return errRegistrarEnrollment
			}
			totals[e.PrincipalTupleDigest]++
		default:
			return errRegistrarEnrollment
		}
		if e.Phase != "committed" && (e.ReceiptID != "" || e.ReceiptDigest != "") {
			return errRegistrarEnrollment
		}
		if e.Phase == "source-work-reserved" || e.Phase == "source-observed-principal-debited" {
			if e.ChallengeDigest != "" || e.ChallengeExpiresAtMS != 0 || e.AccessExpiresAtMS != 0 {
				return errRegistrarEnrollment
			}
		}
		if e.Phase == "challenge-issued" || e.Phase == "committed" {
			if !registrarCredentialSecret(e.ChallengeDigest) || e.ChallengeExpiresAtMS <= e.CheckedAtMS || e.AccessExpiresAtMS != e.ChallengeExpiresAtMS || e.ChallengeExpiresAtMS > e.ReserveDeadlineMS {
				return errRegistrarEnrollment
			}
		}
		if e.Phase == "committed" {
			if !registrarCredentialSecret(e.ReceiptID) || !registrarCredentialSecret(e.ReceiptDigest) {
				return errRegistrarEnrollment
			}
			found := false
			if p.s.data.Registrar != nil {
				for _, r := range p.s.data.Registrar.Records {
					if r.ReceiptID == e.ReceiptID && r.ReceiptDigest == e.ReceiptDigest && r.Scope.AttemptID == key {
						if r.Consumption == nil {
							return errRegistrarEnrollment
						}
						c := r.Consumption.Challenge
						b := c.Binding
						if e.ChallengeDigest != admissionDigest(c) || e.PrincipalID != b.PrincipalID || e.PrincipalNamespace != b.PrincipalNamespace || e.BootstrapDigest != b.CredentialDigest || e.DescriptorDigest != b.DescriptorDigest || e.CheckedAtMS != b.CheckedAtMS || e.ReserveDeadlineMS != b.ReserveDeadlineMS || e.AccessExpiresAtMS != b.AccessUntilMS || e.ChallengeExpiresAtMS != c.ExpiresAtMS || b.PrincipalAuthority != source.Sources[0].PrincipalAuthority {
							return errRegistrarEnrollment
						}
						if committed[key] {
							return errRegistrarEnrollment
						}
						committed[key] = true
						found = true
					}
				}
			}
			if !found {
				return errRegistrarEnrollment
			}
		}
		raw, err := json.MarshalIndent(e, "        ", "  ")
		if err != nil || len(raw) > developmentLedgerEntryCap {
			return errRegistrarEnrollment
		}
	}
	if p.s.data.Registrar != nil {
		for _, r := range p.s.data.Registrar.Records {
			if !committed[r.Scope.AttemptID] {
				return errRegistrarEnrollment
			}
		}
	}
	if len(totals) != len(l.PrincipalTotals) {
		return errRegistrarEnrollment
	}
	for key, n := range totals {
		if n < 1 || n > 4 || l.PrincipalTotals[key] != n {
			return errRegistrarEnrollment
		}
	}
	raw, e := json.MarshalIndent(l, "  ", "  ")
	if e != nil || len(raw) > developmentLedgerReserve {
		return errRegistrarEnrollment
	}
	return nil
}
func validAdmissionPrincipal(s string) bool {
	if len(s) == 0 || len(s) > 256 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
func (p *developmentAdmissionV2) publish(next *registrarAdmissionLedger) error {
	old := p.s.data.Admission
	p.s.data.Admission = next
	if e := p.validate(); e != nil {
		p.s.data.Admission = old
		return e
	}
	return p.s.commitLocked(func() { p.s.data.Admission = old })
}
func (p *developmentAdmissionV2) clone() *registrarAdmissionLedger {
	raw, _ := json.Marshal(p.s.data.Admission)
	var next registrarAdmissionLedger
	json.Unmarshal(raw, &next)
	return &next
}
func (p *developmentAdmissionV2) capacity(o *registrarEnrollmentOwner) error {
	old := p.s.data.Admission
	p.s.data.Admission = nil
	raw, e := json.MarshalIndent(&p.s.data, "", "  ")
	p.s.data.Admission = old
	cap := o.config.MaxStateBytes
	if cap > developmentV2ImageCap {
		cap = developmentV2ImageCap
	}
	b := len(raw) + len(developmentV2Header)
	if e != nil || b > cap || developmentLedgerReserve > cap-b {
		return errRegistrarEnrollment
	}
	remaining := cap - b - developmentLedgerReserve
	if o.live+1 > remaining/registrarEntryByteReserve {
		return errRegistrarEnrollment
	}
	return nil
}
func (p *developmentAdmissionV2) reserve(request contract.SourceReserveRequest, o *registrarEnrollmentOwner) error {
	if e := p.checkConfig(); e != nil {
		return e
	}
	if e := p.capacity(o); e != nil {
		return e
	}
	l := p.s.data.Admission
	if l.SourceTotal >= 8 || l.Entries[request.Launch.AttemptID] != nil {
		return errRegistrarEnrollment
	}
	descriptor, e := contract.ServiceReserveDescriptorDigest(request, o.config.Policy.ServiceID)
	if e != nil {
		return e
	}
	next := p.clone()
	next.SourceTotal++
	next.Entries[request.Launch.AttemptID] = &registrarAdmissionEntry{Version: 2, Purpose: "mnet-initial-enrollment-v2", AttemptID: request.Launch.AttemptID, DescriptorDigest: descriptor, RequestDigest: admissionDigest(request), BootstrapDigest: request.Launch.CredentialDigest, ConfigFingerprint: l.ConfigFingerprint, SourceAuthorityDigest: l.SourceAuthorityDigest, Phase: "source-work-reserved", SourceDebit: 1}
	return p.publish(next)
}
func (p *developmentAdmissionV2) observe(request contract.SourceReserveRequest, observed contract.SourceReserve, o *registrarEnrollmentOwner) error {
	if e := p.checkConfig(); e != nil {
		return e
	}
	key := admissionDigest([]string{observed.Provenance.PrincipalAuthority, observed.Provenance.PrincipalNamespace, observed.Provenance.PrincipalID})
	l := p.s.data.Admission
	if l.PrincipalTotals[key] >= 4 {
		return errRegistrarEnrollment
	}
	entry := l.Entries[request.Launch.AttemptID]
	if entry == nil || entry.Phase != "source-work-reserved" || entry.RequestDigest != admissionDigest(request) {
		return errRegistrarEnrollment
	}
	checked, e := time.Parse(time.RFC3339Nano, observed.CheckedAt)
	if e != nil {
		return e
	}
	deadline, e := time.Parse(time.RFC3339Nano, observed.Deadline)
	if e != nil {
		return e
	}
	next := p.clone()
	n := next.Entries[request.Launch.AttemptID]
	n.Phase = "source-observed-principal-debited"
	n.PrincipalID = observed.Provenance.PrincipalID
	n.PrincipalNamespace = observed.Provenance.PrincipalNamespace
	n.PrincipalTupleDigest = key
	n.PrincipalDebit = 1
	n.ObservationDigest = admissionDigest(observed)
	n.CheckedAtMS = checked.UnixMilli()
	n.ReserveDeadlineMS = deadline.UnixMilli()
	next.PrincipalTotals[key]++
	return p.publish(next)
}
func (p *developmentAdmissionV2) challenge(request contract.SourceReserveRequest, c contract.Challenge, o *registrarEnrollmentOwner) error {
	if e := p.checkConfig(); e != nil {
		return e
	}
	next := p.clone()
	n := next.Entries[request.Launch.AttemptID]
	if n == nil || n.Phase != "source-observed-principal-debited" {
		return errRegistrarEnrollment
	}
	n.Phase = "challenge-issued"
	n.ChallengeDigest = admissionDigest(c)
	n.ChallengeExpiresAtMS = c.ExpiresAtMS
	n.AccessExpiresAtMS = c.Binding.AccessUntilMS
	return p.publish(next)
}
func (p *developmentAdmissionV2) commit(c contract.Challenge, r contract.ReceiptV2) (func(), error) {
	// Caller checked pin before installing candidate registrar state. Validate
	// the whole committed candidate below, including reverse record correlation.
	old := p.s.data.Admission
	next := p.clone()
	n := next.Entries[c.Binding.AttemptID]
	if n == nil || n.Phase != "challenge-issued" || n.ChallengeDigest != admissionDigest(c) {
		return nil, errRegistrarEnrollment
	}
	n.Phase = "committed"
	n.ReceiptID = r.ReceiptID
	n.ReceiptDigest = contract.ReceiptV2Digest(r)
	p.s.data.Admission = next
	if e := p.validate(); e != nil {
		p.s.data.Admission = old
		return nil, e
	}
	return func() { p.s.data.Admission = old }, nil
}

// stage-one refusal/source-work exhaustion is permanent for this root. Bad
// tickets can exhaust it. Restart leaves pending entries spent and stranded.

func (developmentV2Codec) validateAdmission(s *Store) error {
	if s.data.Admission == nil {
		if len(s.data.Nodes) != 0 || s.data.Registrar != nil || len(s.data.AuthKeys) != 0 || len(s.data.NodeTokens) != 0 || len(s.data.ApprovalIdentities) != 0 || len(s.data.Memberships) != 0 {
			return errRegistrarEnrollment
		}
		return nil
	}
	return (&developmentAdmissionV2{s}).validate()
}

func admissionV2PayloadExact(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	t := reflect.TypeFor[storeData]()
	for i := 0; i < t.NumField(); i++ {
		allowed[strings.Split(t.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for d.More() {
		token, e = d.Token()
		name, ok := token.(string)
		if e != nil || !ok || seen[name] || !allowed[name] {
			return false
		}
		seen[name] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
		if name == "admission_ledger" {
			item := json.NewDecoder(bytes.NewReader(value))
			item.UseNumber()
			if !admissionV2Exact(item, reflect.TypeFor[registrarAdmissionLedger](), 0) {
				return false
			}
			if _, e = item.Token(); e != io.EOF {
				return false
			}
		}
	}
	token, e = d.Token()
	if e != nil || token != json.Delim('}') {
		return false
	}
	if _, e = d.Token(); e != io.EOF {
		return false
	}
	return true
}
func admissionV2Exact(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 8 {
		return false
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		token, e := d.Token()
		if e != nil || token != json.Delim('{') {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			fields[t.Field(i).Tag.Get("json")] = t.Field(i).Type
		}
		seen := map[string]bool{}
		for d.More() {
			token, e = d.Token()
			key, ok := token.(string)
			field, exists := fields[key]
			if e != nil || !ok || !exists || seen[key] || !admissionV2Exact(d, field, depth+1) {
				return false
			}
			seen[key] = true
		}
		token, e = d.Token()
		return e == nil && token == json.Delim('}') && len(seen) == len(fields)
	case reflect.Map:
		token, e := d.Token()
		if e != nil || token != json.Delim('{') {
			return false
		}
		seen := map[string]bool{}
		for d.More() {
			token, e = d.Token()
			key, ok := token.(string)
			if e != nil || !ok || seen[key] || len(seen) >= 8 || !admissionV2Exact(d, t.Elem(), depth+1) {
				return false
			}
			seen[key] = true
		}
		token, e = d.Token()
		return e == nil && token == json.Delim('}')
	case reflect.String:
		token, e := d.Token()
		_, ok := token.(string)
		return e == nil && ok
	case reflect.Int, reflect.Int64:
		token, e := d.Token()
		v, ok := token.(json.Number)
		if e != nil || !ok {
			return false
		}
		_, e = strconv.ParseInt(string(v), 10, 64)
		return e == nil
	}
	return false
}

// DisposableAdmissionInspection is local accounting only, never a resume permit,
// readiness assertion, remote-effect certification or refund balance. No raw
// principal, configuration verifier, request, proof or receipt is disclosed.
type DisposableAdmissionInspection struct {
	SourceWorkSpent   int
	PrincipalDebits   int
	CommittedHistory  int
	StrandedPrecommit int
}

// InspectAdmission derives stranded state without rewriting durable phases. On
// owning reopen every precommit entry lacks its original private RAM owner.
// No HTTP endpoint or automatic retry is introduced by this inspection method.
func (d *DisposableRegistrar) InspectAdmission() (DisposableAdmissionInspection, error) {
	var out DisposableAdmissionInspection
	d.store.mu.Lock()
	defer d.store.mu.Unlock()
	p, ok := d.store.admissionPolicy.(*developmentAdmissionV2)
	if !ok {
		return out, errRegistrarEnrollment
	}
	if e := d.store.healthLocked(); e != nil {
		return out, e
	}
	if e := p.checkConfig(); e != nil {
		return out, e
	}
	l := d.store.data.Admission
	out.SourceWorkSpent = l.SourceTotal
	for _, n := range l.PrincipalTotals {
		out.PrincipalDebits += n
	}
	for id, e := range l.Entries {
		if e.Phase == "committed" {
			out.CommittedHistory++
			continue
		}
		owned := false
		if o := d.store.registrarEnrollment; o != nil {
			if a := o.attempts[id]; a != nil {
				owned = a.state == "source-may-have-started" || (a.state == "challenge" && a.proof != nil)
			}
		}
		if !owned {
			out.StrandedPrecommit++
		}
	}
	return out, nil
}
