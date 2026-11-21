package enrollmentconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"unicode/utf8"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/enrollmentidentity"
)

// LocalConfigV2 is a NEW explicit, inactive config schema, not an upgrade of
// issuer/subject Config v1. Source origin, local principal authority and issuer
// label have distinct meanings even when their URL bytes happen to match.
type LocalConfigV2 struct {
	Version            int              `json:"version"`
	Revision           string           `json:"revision"`
	SourceOrigin       string           `json:"source_origin"`
	PrincipalAuthority string           `json:"principal_authority"`
	PrincipalNamespace string           `json:"principal_namespace"`
	IssuerLabel        string           `json:"issuer_label"`
	RegistrarOrigin    string           `json:"registrar_origin"`
	Audience           string           `json:"audience"`
	ServiceID          string           `json:"service_id"`
	Bindings           []LocalBindingV2 `json:"bindings"`
}
type LocalBindingV2 struct {
	WorkspaceID  string   `json:"workspace_id"`
	NetworkID    string   `json:"network_id"`
	PrincipalIDs []string `json:"principal_ids"`
}

func (LocalConfigV2) String() string   { return "[private inactive local enrollment policy DATA]" }
func (LocalConfigV2) GoString() string { return "[private inactive local enrollment policy DATA]" }

// LocalPolicyV2 retains a defensive immutable copy. It is DATA policy syntax,
// not proof this policy is installed/current or that any person is eligible.
// It has no file/environment loader, service credentials, HTTP or authority hook.
type LocalPolicyV2 struct {
	config      LocalConfigV2
	byWorkspace map[string]LocalBindingV2
}

func (LocalPolicyV2) String() string   { return "[private inactive local enrollment policy]" }
func (LocalPolicyV2) GoString() string { return "[private inactive local enrollment policy]" }
func ParseLocalV2(raw []byte) (*LocalPolicyV2, error) {
	if len(raw) == 0 || len(raw) > MaxBytes || !utf8.Valid(raw) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !exactValue(d, reflect.TypeFor[LocalConfigV2](), 0) {
		return nil, ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	var c LocalConfigV2
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, ErrInvalid
	}
	return ValidateLocalV2(c)
}
func ValidateLocalV2(c LocalConfigV2) (*LocalPolicyV2, error) {
	if c.Version != 2 || c.PrincipalNamespace != contract.PrincipalNamespace || !enrollmentidentity.ValidOrigin(c.SourceOrigin) || !enrollmentidentity.ValidOrigin(c.PrincipalAuthority) || !enrollmentidentity.ValidOrigin(c.RegistrarOrigin) || !enrollmentidentity.ValidIssuer(c.IssuerLabel) || !text(c.Revision, 128) || !text(c.Audience, 256) || !localID(c.ServiceID) || len(c.Bindings) == 0 || len(c.Bindings) > 20 {
		return nil, ErrInvalid
	}
	c.Bindings = append([]LocalBindingV2(nil), c.Bindings...)
	p := &LocalPolicyV2{byWorkspace: map[string]LocalBindingV2{}}
	for i, b := range c.Bindings {
		mapping := localSourceMapping(c, b)
		if contract.CheckSourceMapping(mapping) != nil || len(b.PrincipalIDs) == 0 || len(b.PrincipalIDs) > 100 {
			return nil, ErrInvalid
		}
		if _, duplicate := p.byWorkspace[b.WorkspaceID]; duplicate {
			return nil, ErrInvalid
		}
		seen := map[string]bool{}
		b.PrincipalIDs = append([]string(nil), b.PrincipalIDs...)
		for _, id := range b.PrincipalIDs {
			if !text(id, 256) || seen[id] {
				return nil, ErrInvalid
			}
			seen[id] = true
		}
		c.Bindings[i] = b
		p.byWorkspace[b.WorkspaceID] = b
	}
	p.config = c
	return p, nil
}
func localID(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for i, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || ((c == '_' || c == '-') && i > 0) {
			continue
		}
		return false
	}
	return true
}
func localSourceMapping(c LocalConfigV2, b LocalBindingV2) contract.SourceMapping {
	return contract.SourceMapping{WorkspaceID: b.WorkspaceID, Issuer: c.IssuerLabel, RegistrarOrigin: c.RegistrarOrigin, Audience: c.Audience, MappingRevision: c.Revision, NetworkID: b.NetworkID}
}

// Snapshot returns a defensive policy DATA copy, never an installed-policy token.
func (p *LocalPolicyV2) Snapshot() (LocalConfigV2, bool) {
	if p == nil || p.config.Version != 2 {
		return LocalConfigV2{}, false
	}
	c := p.config
	c.Bindings = append([]LocalBindingV2(nil), c.Bindings...)
	for i := range c.Bindings {
		c.Bindings[i].PrincipalIDs = append([]string(nil), c.Bindings[i].PrincipalIDs...)
	}
	return c, true
}
func (p *LocalPolicyV2) SourceMappings() []contract.SourceMapping {
	if p == nil || p.config.Version != 2 {
		return nil
	}
	out := make([]contract.SourceMapping, 0, len(p.config.Bindings))
	for _, b := range p.config.Bindings {
		out = append(out, localSourceMapping(p.config, b))
	}
	return out
}

// AllowsObservedDATA compares explicit local namespace/principal/service/mapping
// against this syntax-checked snapshot. It authenticates none of its inputs and
// does NOT check current source eligibility/clock/revocation or issue a permit.
func (p *LocalPolicyV2) AllowsObservedDATA(mapping contract.SourceMapping, authority, namespace, principalID, serviceID string) bool {
	if p == nil || p.config.Version != 2 || authority != p.config.PrincipalAuthority || namespace != p.config.PrincipalNamespace || serviceID != p.config.ServiceID {
		return false
	}
	b, ok := p.byWorkspace[mapping.WorkspaceID]
	if !ok || mapping != localSourceMapping(p.config, b) {
		return false
	}
	for _, id := range b.PrincipalIDs {
		if id == principalID {
			return true
		}
	}
	return false
}
