package control

// Persisted registrar groundwork only. There is deliberately no constructor,
// permit issuer, commit method or credential verifier. These private records
// can only be loaded/validated or revoked by the existing owning lifecycle.
import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"fintex.au/mnet/internal/enrollmentidentity"
)

var errRegistrarState = errors.New("store: invalid registrar state; file was not changed")

type registrarState struct {
	Version int                         `json:"version"`
	Records map[string]*registrarRecord `json:"records"`
}
type registrarScope struct {
	Issuer          string `json:"issuer"`
	RegistrarOrigin string `json:"registrar_origin"`
	Audience        string `json:"audience"`
	ServiceID       string `json:"service_id"`
	WorkspaceID     string `json:"workspace_id"`
	MappingRevision string `json:"mapping_revision"`
	HandoffID       string `json:"handoff_id"`
	AttemptID       string `json:"attempt_id"`
	ReserveID       string `json:"reserve_id"`
}
type registrarRecord struct {
	NetworkID                 string         `json:"network_id"`
	DevicePublicKey           string         `json:"device_public_key"`
	NodeID                    string         `json:"node_id"`
	CredentialDigest          string         `json:"credential_digest"`
	Phase                     string         `json:"phase"`
	Scope                     registrarScope `json:"scope"`
	DescriptorDigest          string         `json:"descriptor_digest"`
	DescriptorEncodingVersion int            `json:"descriptor_encoding_version"`
	ReceiptEncodingVersion    int            `json:"receipt_encoding_version"`
	ReceiptID                 string         `json:"receipt_id"`
	ReceiptDigest             string         `json:"receipt_digest"`
	CommittedAtMS             int64          `json:"committed_at_ms"`
	AccessExpiresAtMS         int64          `json:"access_expires_at_ms"`
}

// No escaping, normalization or JSON serialization participates in receipt v1.
// Every field is uint32 BE byte length + exact bytes, including the two times
// represented as uint64 BE positive Unix milliseconds. Phase is mutable history
// and deliberately not in the immutable receipt. No raw credential is included.
func registrarReceiptBytes(r registrarRecord) []byte {
	fields := [][]byte{}
	for _, s := range []string{"mnet/store-enrollment-receipt/v1", r.Scope.Issuer, r.Scope.RegistrarOrigin, r.Scope.Audience, r.Scope.ServiceID, r.Scope.WorkspaceID, r.Scope.MappingRevision, r.Scope.HandoffID, r.Scope.AttemptID, r.Scope.ReserveID, r.DescriptorDigest, "1", r.ReceiptID, r.NodeID, r.NetworkID, r.DevicePublicKey, r.CredentialDigest} {
		fields = append(fields, []byte(s))
	}
	for _, n := range []int64{r.CommittedAtMS, r.AccessExpiresAtMS} {
		fields = append(fields, binary.BigEndian.AppendUint64(nil, uint64(n)))
	}
	var out []byte
	for _, f := range fields {
		out = binary.BigEndian.AppendUint32(out, uint32(len(f)))
		out = append(out, f...)
	}
	return out
}
func registrarReceiptDigest(r registrarRecord) string {
	d := sha256.Sum256(registrarReceiptBytes(r))
	return base64.RawURLEncoding.EncodeToString(d[:])
}
func registrarClaimKey(network, key string) string {
	b, _ := json.Marshal([]string{network, key})
	return string(b)
}
func registrarText(s string) bool {
	return s != "" && len(s) <= 2048 && utf8.ValidString(s) && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}
func registrarDigest(s string) bool {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}

// Duplicate keys are rejected recursively inside the new security namespace.
func registrarUniqueJSON(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return errRegistrarState
			}
			seen[s] = true
			if err := registrarUniqueJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := registrarUniqueJSON(d); err != nil {
				return err
			}
		}
	default:
		return errRegistrarState
	}
	_, err = d.Token()
	return err
}
func (r *registrarState) UnmarshalJSON(b []byte) error {
	if !utf8.Valid(b) || bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return errRegistrarState
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if err := registrarUniqueJSON(d); err != nil {
		return errRegistrarState
	}
	type plain registrarState
	var value plain
	if !registrarExactFields(b, []string{"version", "records"}) {
		return errRegistrarState
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(b, &root) != nil {
		return errRegistrarState
	}
	var records map[string]json.RawMessage
	if json.Unmarshal(root["records"], &records) != nil {
		return errRegistrarState
	}
	for _, raw := range records {
		if !registrarExactFields(raw, []string{"network_id", "device_public_key", "node_id", "credential_digest", "phase", "scope", "descriptor_digest", "descriptor_encoding_version", "receipt_encoding_version", "receipt_id", "receipt_digest", "committed_at_ms", "access_expires_at_ms"}) {
			return errRegistrarState
		}
		var record map[string]json.RawMessage
		if json.Unmarshal(raw, &record) != nil {
			return errRegistrarState
		}
		if !registrarExactFields(record["scope"], []string{"issuer", "registrar_origin", "audience", "service_id", "workspace_id", "mapping_revision", "handoff_id", "attempt_id", "reserve_id"}) {
			return errRegistrarState
		}
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return errRegistrarState
	}
	if value.Version != 1 || value.Records == nil {
		return errRegistrarState
	}
	*r = registrarState(value)
	return nil
}

// Pointer null cannot be distinguished from omission by encoding/json alone.
// Also reject duplicate root registrar_state entries without redefining legacy
// unknown-field handling. Other unknown legacy top-level fields stay compatible.
func validateRegistrarEnvelope(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return errRegistrarState
	}
	found := false
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return errRegistrarState
		}
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return errRegistrarState
		}
		if name, ok := key.(string); ok && strings.EqualFold(name, "registrar_state") {
			if name != "registrar_state" {
				return errRegistrarState
			}
			if found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return errRegistrarState
			}
			found = true
		}
	}
	if _, err = d.Token(); err != nil {
		return errRegistrarState
	}
	if _, err = d.Token(); err != io.EOF {
		return errRegistrarState
	}
	return nil
}

func (s *Store) validateRegistrarState() error {
	state := s.data.Registrar
	if state == nil {
		return nil
	}
	if state.Version != 1 || state.Records == nil {
		return errRegistrarState
	}
	nodes := map[string]bool{}
	identities := map[string]bool{}
	for index, r := range state.Records {
		if r == nil || index != registrarClaimKey(r.NetworkID, r.DevicePublicKey) || !validPubKey(r.DevicePublicKey) || !registrarText(r.NetworkID) || !registrarText(r.NodeID) || nodes[r.NodeID] {
			return errRegistrarState
		}
		nodes[r.NodeID] = true
		if r.Phase != "active" && r.Phase != "revoked" {
			return errRegistrarState
		}
		if r.DescriptorEncodingVersion != 1 || r.ReceiptEncodingVersion != 1 || r.CommittedAtMS <= 0 || r.AccessExpiresAtMS <= r.CommittedAtMS {
			return errRegistrarState
		}
		if !enrollmentidentity.ValidIssuer(r.Scope.Issuer) || !enrollmentidentity.ValidOrigin(r.Scope.RegistrarOrigin) {
			return errRegistrarState
		}
		for _, text := range []string{r.Scope.Audience, r.Scope.ServiceID, r.Scope.WorkspaceID, r.Scope.MappingRevision} {
			if !registrarText(text) {
				return errRegistrarState
			}
		}
		for _, digest := range []string{r.CredentialDigest, r.DescriptorDigest, r.ReceiptID, r.ReceiptDigest, r.Scope.HandoffID, r.Scope.AttemptID, r.Scope.ReserveID} {
			if !registrarDigest(digest) {
				return errRegistrarState
			}
		}
		if registrarReceiptDigest(*r) != r.ReceiptDigest {
			return errRegistrarState
		}
		// No duplicate handoff/attempt/reserve under the same issuer and registrar,
		// even when other echoed fields/service IDs have been changed.
		for _, item := range [][2]string{{"handoff", r.Scope.HandoffID}, {"attempt", r.Scope.AttemptID}, {"reserve", r.Scope.ReserveID}} {
			b, _ := json.Marshal([]string{r.Scope.Issuer, r.Scope.RegistrarOrigin, item[0], item[1]})
			key := string(b)
			if identities[key] {
				return errRegistrarState
			}
			identities[key] = true
		}
		if _, mixed := s.data.NodeTokens[r.NodeID]; mixed {
			return errRegistrarState
		}
		if _, mixed := s.data.ApprovalIdentities[r.NodeID]; mixed {
			return errRegistrarState
		}
		n := s.data.Nodes[r.NodeID]
		if n == nil {
			if r.Phase == "active" {
				return errRegistrarState
			}
			continue
		}
		if n.ID != r.NodeID || n.NetworkID != r.NetworkID || n.PubKey != r.DevicePublicKey || s.data.Networks[r.NetworkID] == nil || n.KeyExpiry == nil || n.KeyExpiry.UnixMilli() > r.AccessExpiresAtMS || n.ExitNode || len(n.Routes) != 0 || n.User != "" {
			return errRegistrarState
		}
		if r.Phase == "active" && (!n.Approved || n.KeyExpiry.UnixMilli() != r.AccessExpiresAtMS) {
			return errRegistrarState
		}
	}
	// An alternate node cannot reuse a claimed raw-key identity.
	for id, n := range s.data.Nodes {
		if r := state.Records[registrarClaimKey(n.NetworkID, n.PubKey)]; r != nil && id != r.NodeID {
			return errRegistrarState
		}
	}
	return nil
}
func (s *Store) registrarClaimLocked(network, key string) *registrarRecord {
	if s.data.Registrar == nil {
		return nil
	}
	return s.data.Registrar.Records[registrarClaimKey(network, key)]
}
func (s *Store) registrarNodeLocked(id string) *registrarRecord {
	if s.data.Registrar == nil {
		return nil
	}
	for _, r := range s.data.Registrar.Records {
		if r.NodeID == id {
			return r
		}
	}
	return nil
}
func (s *Store) registrarAccessDeniedLocked(id string) bool {
	r := s.registrarNodeLocked(id)
	return r != nil && (r.Phase != "active" || time.Now().UnixMilli() >= r.AccessExpiresAtMS)
}

func registrarExactFields(raw []byte, fields []string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != len(fields) {
		return false
	}
	for _, key := range fields {
		value, ok := object[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return true
}
