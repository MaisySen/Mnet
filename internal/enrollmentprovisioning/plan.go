// Package enrollmentprovisioning checks disconnected provisioning DATA only.
// It has no network, loader, secret, file writer, membership, installed-policy,
// receipt, daemon, service or authority dependency. A valid plan is NOT consent.
package enrollmentprovisioning

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"sort"
	"unicode/utf8"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
)

const MaxBytes = 256 * 1024

var ErrPlan = errors.New("disconnected provisioning plan invalid or inconsistent; no authority or configuration was created")
var id = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// OwnerRef and TrustRootSHA256 are unverified declarations for later recovery
// of original approvals/independent installed trust. They authenticate nothing.
type Plan struct {
	Version          int               `json:"version"`
	Purpose          string            `json:"purpose"`
	LegacyBoundary   string            `json:"proposed_legacy_boundary"`
	Change           string            `json:"proposed_change"`
	Service          ServiceSnapshot   `json:"multies_service_scope_snapshot"`
	OperatorOwnerRef string            `json:"operator_owner_ref"`
	Multies          OperatorSnapshot  `json:"multies_operator_snapshot"`
	Registrar        RegistrarSnapshot `json:"registrar_source_snapshot"`
	Native           NativeSnapshot    `json:"native_policy_snapshot"`
	WorkspaceOwners  []WorkspaceOwner  `json:"workspace_owner_refs"`
}
type OperatorSnapshot struct {
	Provenance      Provenance        `json:"provenance"`
	Revision        string            `json:"revision"`
	Issuer          string            `json:"issuer"`
	RegistrarOrigin string            `json:"registrarOrigin"`
	Audience        string            `json:"audience"`
	Bindings        []OperatorBinding `json:"bindings"`
}
type ServiceSnapshot struct {
	KeyID           string `json:"keyId"`
	ServiceID       string `json:"serviceId"`
	RegistrarOrigin string `json:"registrarOrigin"`
	Issuer          string `json:"issuer"`
	Audience        string `json:"audience"`
}
type Provenance struct {
	Version   int    `json:"version"`
	Authority string `json:"principalAuthority"`
	Namespace string `json:"principalNamespace"`
}
type OperatorBinding struct {
	Workspace  string   `json:"workspaceId"`
	Network    string   `json:"networkId"`
	Principals []string `json:"principals"`
}

// Deliberately NOT registrarSourceEntry: that installed schema contains a
// service bearer. This secret-free reference is noninstallable plan DATA.
type RegistrarSnapshot struct {
	SourceOrigin       string                   `json:"source_origin"`
	PrincipalAuthority string                   `json:"principal_authority"`
	KeyID              string                   `json:"key_id"`
	ServiceID          string                   `json:"service_id"`
	Issuer             string                   `json:"issuer_label"`
	RegistrarOrigin    string                   `json:"registrar_origin"`
	Audience           string                   `json:"audience"`
	SecretOwnerRef     string                   `json:"secret_owner_ref"`
	Allowed            []contract.SourceMapping `json:"allowed"`
}
type NativeSnapshot struct {
	// This candidate requires a separately declared reference to independently
	// installed trust, not a key supplied by the registrar or an unsigned URL.
	TrustOwnerRef   string               `json:"independent_trust_owner_ref"`
	TrustRootSHA256 string               `json:"independent_trust_root_sha256"`
	Policy          config.LocalConfigV2 `json:"policy"`
}
type WorkspaceOwner struct {
	Workspace string `json:"workspace_id"`
	OwnerRef  string `json:"owner_ref"`
}
type Check struct {
	Owner       string `json:"owner"`
	Requirement string `json:"requirement"`
	Status      string `json:"status"`
}
type Report struct {
	Version                 int    `json:"version"`
	Kind                    string `json:"kind"`
	ConfigurationConsistent bool   `json:"configuration_consistent"`
	AuthorityVerified       bool   `json:"authority_verified"`
	Deployable              bool   `json:"deployable"`
	// Hash is canonical DATA identity only, not signature/trust/approval.
	PlanSHA256     string  `json:"plan_sha256"`
	WorkspaceCount int     `json:"workspace_count"`
	RequiredChecks []Check `json:"required_checks"`
}

// Verify accepts exact bounded JSON and checks three independently SUPPLIED
// snapshots agree. It cannot establish they were independently OBTAINED.
func Verify(raw []byte) (Report, error) {
	var empty Report
	if len(raw) == 0 || len(raw) > MaxBytes || !utf8.Valid(raw) {
		return empty, ErrPlan
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !shape(d, reflect.TypeFor[Plan](), 0) {
		return empty, ErrPlan
	}
	if _, e := d.Token(); e != io.EOF {
		return empty, ErrPlan
	}
	var p Plan
	if json.Unmarshal(raw, &p) != nil {
		return empty, ErrPlan
	}
	p, e := validate(p)
	if e != nil {
		return empty, e
	}
	canonical, e := json.Marshal(p)
	if e != nil {
		return empty, ErrPlan
	}
	h := sha256.Sum256(canonical)
	checks := []Check{
		{p.OperatorOwnerRef, "authenticate operator ownership, actual source/registrar origins and service scope; no config installed", "unverified"},
		{p.Native.TrustOwnerRef, "verify this exact root fingerprint through independent installed trust; plan cannot bootstrap its own trust", "unverified"},
		{p.Registrar.SecretOwnerRef, "resolve server-only service credential in protected storage and scope/rotation; no secret accepted by this model", "unverified"},
	}
	for _, o := range p.WorkspaceOwners {
		checks = append(checks, Check{o.OwnerRef, "verify current authenticated workspace ownership and explicit workspace-to-network policy for " + o.Workspace + "; principal allowlist is not membership", "unverified"})
	}
	checks = append(checks, Check{p.OperatorOwnerRef, "enforce proposed registrar-only Multies network namespace across legacy join/account/OIDC/admin/map/relay; inventory existing legacy nodes and separately approve any migration/retirement; current server does NOT enforce this boundary", "unresolved"}, Check{p.OperatorOwnerRef, "initial provisioning only: verify no pending attempts or earlier conflicting scope; key rotation/mapping revision changes require a separate reviewed plan and pending-attempt refusal, not edit/replay of this hash", "unverified"}, Check{"engineering-policy", "review useful finite grants, renewal, revocation/offline residuals, native/version floor and durable server admission; no duration or quota selected", "unresolved"}, Check{"deployment-owner", "verify exact account/repository/domain/storage/billing and obtain deployment/payment/OS-install approvals; no action authorized", "unverified"})
	return Report{Version: 1, Kind: "disconnected-configuration-consistency-only", ConfigurationConsistent: true, AuthorityVerified: false, Deployable: false, PlanSHA256: hex.EncodeToString(h[:]), WorkspaceCount: len(p.Native.Policy.Bindings), RequiredChecks: checks}, nil
}
func validate(p Plan) (Plan, error) {
	if p.LegacyBoundary != "proposed-registrar-only-multies-networks-legacy-separate" || p.Change != "initial-provision-only-no-rotation-or-migration" || p.Version != 1 || p.Purpose != "mnet-disconnected-provisioning-plan-v1" || !id.MatchString(p.OperatorOwnerRef) || !id.MatchString(p.Native.TrustOwnerRef) || !id.MatchString(p.Registrar.SecretOwnerRef) || !digest.MatchString(p.Native.TrustRootSHA256) || p.Native.TrustRootSHA256 == string(bytes.Repeat([]byte{'0'}, 64)) {
		return p, ErrPlan
	}
	validated, e := config.ValidateLocalV2(p.Native.Policy)
	if e != nil {
		return p, ErrPlan
	}
	policy, _ := validated.Snapshot()
	p.Native.Policy = policy
	m, r := p.Multies, p.Registrar
	if m.Provenance.Version != 2 || m.Provenance.Authority != policy.PrincipalAuthority || m.Provenance.Namespace != contract.PrincipalNamespace || !id.MatchString(m.Revision) || m.Revision != policy.Revision || m.Issuer != policy.IssuerLabel || m.RegistrarOrigin != policy.RegistrarOrigin || m.Audience != policy.Audience {
		return p, ErrPlan
	}
	if p.Service.KeyID != r.KeyID || p.Service.ServiceID != r.ServiceID || p.Service.RegistrarOrigin != r.RegistrarOrigin || p.Service.Issuer != r.Issuer || p.Service.Audience != r.Audience {
		return p, ErrPlan
	}
	if r.SourceOrigin != policy.SourceOrigin || r.PrincipalAuthority != policy.PrincipalAuthority || r.Issuer != policy.IssuerLabel || r.RegistrarOrigin != policy.RegistrarOrigin || r.Audience != policy.Audience || r.ServiceID != policy.ServiceID || !id.MatchString(r.KeyID) {
		return p, ErrPlan
	}
	if len(m.Bindings) != len(policy.Bindings) || len(r.Allowed) != len(policy.Bindings) || len(p.WorkspaceOwners) != len(policy.Bindings) {
		return p, ErrPlan
	}
	// Sort only defensive data from parsing; never mutate caller-owned objects.
	sort.Slice(p.Native.Policy.Bindings, func(i, j int) bool {
		return p.Native.Policy.Bindings[i].WorkspaceID < p.Native.Policy.Bindings[j].WorkspaceID
	})
	sort.Slice(p.Multies.Bindings, func(i, j int) bool { return p.Multies.Bindings[i].Workspace < p.Multies.Bindings[j].Workspace })
	sort.Slice(p.Registrar.Allowed, func(i, j int) bool { return p.Registrar.Allowed[i].WorkspaceID < p.Registrar.Allowed[j].WorkspaceID })
	sort.Slice(p.WorkspaceOwners, func(i, j int) bool { return p.WorkspaceOwners[i].Workspace < p.WorkspaceOwners[j].Workspace })
	seenNetworks := map[string]bool{}
	for i, b := range p.Native.Policy.Bindings {
		if seenNetworks[b.NetworkID] {
			return p, ErrPlan
		}
		seenNetworks[b.NetworkID] = true
		a := p.Multies.Bindings[i]
		mapping := p.Registrar.Allowed[i]
		owner := p.WorkspaceOwners[i]
		expected := contract.SourceMapping{WorkspaceID: b.WorkspaceID, NetworkID: b.NetworkID, Issuer: policy.IssuerLabel, RegistrarOrigin: policy.RegistrarOrigin, Audience: policy.Audience, MappingRevision: policy.Revision}
		if a.Workspace != b.WorkspaceID || a.Network != b.NetworkID || mapping != expected || owner.Workspace != b.WorkspaceID || !id.MatchString(owner.OwnerRef) || len(a.Principals) != len(b.PrincipalIDs) {
			return p, ErrPlan
		}
		sort.Strings(p.Native.Policy.Bindings[i].PrincipalIDs)
		sort.Strings(p.Multies.Bindings[i].Principals)
		for j, principal := range p.Native.Policy.Bindings[i].PrincipalIDs {
			if p.Multies.Bindings[i].Principals[j] != principal {
				return p, ErrPlan
			}
		}
	}
	return p, nil
}

// Exact required fields, no aliases/duplicates/unknown/null/floats, bounded depth
// and collection size. All snapshots are DATA: no external instruction is run.
func shape(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 10 {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		v, e := d.Token()
		if e != nil || v != json.Delim('{') {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fields[f.Tag.Get("json")] = f.Type
		}
		seen := map[string]bool{}
		for d.More() {
			v, e = d.Token()
			name, ok := v.(string)
			if e != nil || !ok || seen[name] {
				return false
			}
			ft, ok := fields[name]
			if !ok {
				return false
			}
			seen[name] = true
			if !shape(d, ft, depth+1) {
				return false
			}
		}
		v, e = d.Token()
		return e == nil && v == json.Delim('}') && len(seen) == len(fields)
	case reflect.Slice:
		v, e := d.Token()
		if e != nil || v != json.Delim('[') {
			return false
		}
		for n := 0; d.More(); n++ {
			if n >= 100 || !shape(d, t.Elem(), depth+1) {
				return false
			}
		}
		v, e = d.Token()
		return e == nil && v == json.Delim(']')
	case reflect.String:
		v, e := d.Token()
		_, ok := v.(string)
		return e == nil && ok
	case reflect.Int:
		v, e := d.Token()
		n, ok := v.(json.Number)
		if e != nil || !ok {
			return false
		}
		_, e = n.Int64()
		return e == nil
	}
	return false
}
