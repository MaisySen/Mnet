package daemon

// Owner-only native credential encoding. Public State snapshots/JSON never
// contain this document; detached snapshots cannot publish native State.
import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/stateowner"
	"io"
	"os"
	"path/filepath"
)

type nativeSavedCredential struct {
	Document              contract.NativeCredentialDocument `json:"document"`
	OriginalStateRevision string                            `json:"original_state_revision"`
	PolicyRevision        string                            `json:"policy_revision"`
}

func (nativeSavedCredential) String() string   { return "[private sole native credential]" }
func (nativeSavedCredential) GoString() string { return "[private sole native credential]" }
func canonicalRevision(s string) bool {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == s
}
func (n nativeSavedCredential) valid() bool {
	return canonicalRevision(n.OriginalStateRevision) && canonicalRevision(n.PolicyRevision) && contract.CheckNativeCredentialDocument(n.Document) == nil
}
func decodeSavedCredential(raw []byte) (*nativeSavedCredential, error) {
	// Strict outer shape through exact RawMessage keys, strict inner contract.
	var fields map[string]json.RawMessage
	if !exactNativeOuter(raw) || json.Unmarshal(raw, &fields) != nil || len(fields) != 3 {
		return nil, ErrStateStale
	}
	var n nativeSavedCredential
	if json.Unmarshal(fields["original_state_revision"], &n.OriginalStateRevision) != nil || json.Unmarshal(fields["policy_revision"], &n.PolicyRevision) != nil {
		return nil, ErrStateStale
	}
	doc, err := contract.DecodeNativeCredentialDocument(fields["document"])
	if err != nil {
		return nil, ErrStateStale
	}
	n.Document = doc
	if !n.valid() {
		return nil, ErrStateStale
	}
	return &n, nil
}
func exactNativeOuter(raw []byte) bool {
	// Token decoder catches duplicates before ordinary map unmarshal.
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		k, err := d.Token()
		name, ok := k.(string)
		if err != nil || !ok || seen[name] || (name != "document" && name != "original_state_revision" && name != "policy_revision") {
			return false
		}
		seen[name] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
	}
	tok, err = d.Token()
	if err != nil || tok != json.Delim('}') {
		return false
	}
	_, err = d.Token()
	return err == io.EOF && len(seen) == 3
}
func marshalOwnedState(s *State) ([]byte, error) {
	type legacy State
	if s.native == nil {
		return json.MarshalIndent((*legacy)(s), "", "  ")
	}
	raw, err := json.Marshal(s.native)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		*legacy
		Native string `json:"native_enrollment_v1"`
	}{(*legacy)(s), string(raw)}, "", "  ")
}
func unmarshalOwnedState(raw []byte, s *State) error {
	type legacy State
	var disk struct {
		*legacy
		Native string `json:"native_enrollment_v1"`
	}
	disk.legacy = (*legacy)(s)
	if err := json.Unmarshal(raw, &disk); err != nil {
		return err
	}
	if disk.Native != "" {
		n, err := decodeSavedCredential([]byte(disk.Native))
		if err != nil {
			return err
		}
		s.native = n
	}
	return s.validateIdentity()
}
func (s *State) validateNativeCredential() error {
	if s.native == nil {
		return nil
	}
	n := s.native
	if !n.valid() || s.NodeID != "" || s.NodeToken != "" || s.AuthKey != "" || s.Server != "" || s.Network != "" || s.ExitNode || len(s.AdvertiseRoutes) != 0 || s.UseExitNode != "" || s.PrivateKey == "" || s.PublicKey == "" {
		return errors.New("mixed or invalid native credential state")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(s.PublicKey)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != n.Document.Challenge.Binding.DevicePublicKey {
		return ErrStateStale
	}
	return nil
}
func publicState(s State) State {
	s = cloneState(s)
	if s.native != nil {
		s.native = nil
		s.nativeDetached = true
	}
	return s
}

// A missing sole State must never cause a replacement key while any earlier
// pending intent survives. A finalization marker with legacy-only State is an
// explicit conflict, not permission to reinterpret or recreate a credential.
// Safety read only, no policy/network/startup activation or journal rewrite.
func guardNativeStateHistory(o *nativeStateOwner) error {
	path := filepath.Join(o.lease.Parent(), "enrollment-pending", "enrollment-pending-v1.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !o.saved.revisionExists {
		return ErrStateStale
	}
	if o.saved.native != nil {
		return nil
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 || stateowner.SingleLink(path, info) != nil {
		return ErrStateStale
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrStateStale
	}
	var version int
	json.Unmarshal(fields["version"], &version)
	if _, exists := fields["finalization_json"]; exists || version == 5 {
		return ErrStateStale
	}
	return nil
}
