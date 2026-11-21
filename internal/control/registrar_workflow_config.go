package control

// Explicit local development workflow policy. No environment/default/startup
// loader or registered route exists. Neither this policy nor a source reserve
// claims immediate revocation or production-ready access lease semantics.
import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"reflect"
	"unicode/utf8"

	config "fintex.au/mnet/internal/enrollmentconfig"
)

type registrarWorkflowConfig struct {
	Version             int                  `json:"version"`
	Mode                string               `json:"mode"`
	Revision            string               `json:"revision"`
	SourceID            string               `json:"source_id"`
	Policy              config.LocalConfigV2 `json:"policy"`
	ChallengeMS         int                  `json:"challenge_ms"`
	MaxLeaseMS          int                  `json:"max_lease_ms"`
	MaxObservationAgeMS int                  `json:"max_observation_age_ms"`
	ClockUncertaintyMS  int                  `json:"clock_uncertainty_ms"`
	MaxLivePerPrincipal int                  `json:"max_live_per_principal"`
	MaxProofAttempts    int                  `json:"max_proof_attempts"`
	MaxLive             int                  `json:"max_live"`
	MaxAttempts         int                  `json:"max_attempts"`
	MaxRecords          int                  `json:"max_records"`
	MaxStateBytes       int                  `json:"max_state_bytes"`
}

func (registrarWorkflowConfig) String() string {
	return "[private development registrar workflow policy]"
}
func (registrarWorkflowConfig) GoString() string {
	return "[private development registrar workflow policy]"
}
func parseRegistrarWorkflow(raw []byte) (registrarWorkflowConfig, *config.LocalPolicyV2, error) {
	var c registrarWorkflowConfig
	if len(raw) == 0 || len(raw) > registrarSourceMaxBytes || !utf8.Valid(raw) {
		return c, nil, errRegistrarSource
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !registrarConfigShape(d, reflect.TypeOf(c), 0) {
		return c, nil, errRegistrarSource
	}
	if _, err := d.Token(); err != io.EOF {
		return c, nil, errRegistrarSource
	}
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Mode != "development" || c.Revision == "" || len(c.Revision) > 128 || c.SourceID == "" {
		return c, nil, errRegistrarSource
	}
	for _, r := range c.Revision {
		if r < 33 || r > 126 {
			return c, nil, errRegistrarSource
		}
	}
	p, err := config.ValidateLocalV2(c.Policy)
	if err != nil || c.ChallengeMS < 1 || c.ChallengeMS > 60000 || c.MaxLeaseMS < 1 || c.MaxLeaseMS > 900000 || c.ChallengeMS > c.MaxLeaseMS || c.MaxObservationAgeMS < 1 || c.MaxObservationAgeMS > 60000 || c.ClockUncertaintyMS < 0 || c.ClockUncertaintyMS >= c.ChallengeMS || c.MaxLivePerPrincipal < 1 || c.MaxLivePerPrincipal > c.MaxLive || c.MaxProofAttempts < 1 || c.MaxProofAttempts > 16 || c.MaxLive < 1 || c.MaxLive > 64 || c.MaxAttempts < c.MaxLive || c.MaxAttempts > 4096 || c.MaxRecords < 1 || c.MaxRecords > 4096 || c.MaxStateBytes < registrarEntryByteReserve || c.MaxStateBytes > 16*1024*1024 {
		return c, nil, errRegistrarSource
	}
	return c, p, nil
}
func (s *Store) registrarWorkflowLocked() (*registrarEnrollmentOwner, error) {
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	old := s.registrarEnrollment
	if old != nil && old.retired {
		return nil, errRegistrarSource
	}
	raw, err := readRegistrarSources(s.ownedPath + ".registrar-workflow-v1.json")
	if err != nil {
		if old != nil {
			old.close()
		}
		return nil, errRegistrarSource
	}
	revision := sha256.Sum256(raw)
	sourceOwner, err := s.registrarSourcesLocked()
	if err != nil || old != nil && (revision != old.revision || old.sources != sourceOwner) {
		if old != nil {
			old.close()
		}
		return nil, errRegistrarSource
	}
	if old != nil {
		return old, nil
	}
	c, p, err := parseRegistrarWorkflow(raw)
	if err != nil {
		return nil, err
	}
	// Resolve alias and all independently configured scopes locally. The source
	// response cannot select policy, registrar, principal namespace or service.
	sourceRaw, err := readRegistrarSources(s.ownedPath + ".registrar-sources-v1.json")
	if err != nil || sha256.Sum256(sourceRaw) != sourceOwner.revision {
		return nil, errRegistrarSource
	}
	var sources registrarSourceConfig
	if json.Unmarshal(sourceRaw, &sources) != nil {
		return nil, errRegistrarSource
	}
	matched := false
	for _, entry := range sources.Sources {
		if entry.ID != c.SourceID {
			continue
		}
		if entry.SourceOrigin != c.Policy.SourceOrigin || entry.PrincipalAuthority != c.Policy.PrincipalAuthority || entry.ServiceID != c.Policy.ServiceID || entry.IssuerLabel != c.Policy.IssuerLabel || entry.RegistrarOrigin != c.Policy.RegistrarOrigin || entry.Audience != c.Policy.Audience {
			return nil, errRegistrarSource
		}
		for _, m := range p.SourceMappings() {
			found := false
			for _, allowed := range entry.Allowed {
				if m == allowed {
					found = true
				}
			}
			if !found {
				return nil, errRegistrarSource
			}
		}
		matched = true
	}
	if !matched {
		return nil, errRegistrarSource
	}
	o := &registrarEnrollmentOwner{revision: revision, sources: sourceOwner, config: c, policy: p, attempts: map[string]*registrarEnrollmentAttempt{}}
	s.registrarEnrollment = o
	return o, nil
}
