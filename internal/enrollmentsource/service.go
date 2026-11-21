package enrollmentsource

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/enrollmentidentity"
)

// ServiceScope is explicit installed-config DATA. NewService is not an operator
// grant; callers cannot pass arbitrary clients into a future authority owner.
type ServiceScope struct{ SourceOrigin, PrincipalAuthority, KeyID, ServiceID, Issuer, RegistrarOrigin, Audience string }
type Service struct {
	scope     ServiceScope
	secret    string
	allowed   []contract.SourceMapping
	http      *http.Client
	transport *http.Transport
}

func (Service) String() string   { return "[private configured enrollment service source client]" }
func (Service) GoString() string { return "[private configured enrollment service source client]" }

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func NewService(scope ServiceScope, secret string, allowed []contract.SourceMapping, limits Limits) (*Service, error) {
	if !enrollmentidentity.ValidOrigin(scope.SourceOrigin) || !enrollmentidentity.ValidOrigin(scope.PrincipalAuthority) || !enrollmentidentity.ValidOrigin(scope.RegistrarOrigin) || !enrollmentidentity.ValidIssuer(scope.Issuer) || !identifierPattern.MatchString(scope.KeyID) || !identifierPattern.MatchString(scope.ServiceID) || !canonicalCode(secret) || len(allowed) == 0 || len(allowed) > 20 || limits.Timeout <= 0 || limits.Timeout > time.Minute || limits.MaxConnections < 1 || limits.MaxConnections > 16 {
		return nil, ErrInvalid
	}
	seen := map[contract.SourceMapping]bool{}
	for _, m := range allowed {
		if contract.CheckSourceMapping(m) != nil || m.Issuer != scope.Issuer || m.RegistrarOrigin != scope.RegistrarOrigin || m.Audience != scope.Audience || seen[m] {
			return nil, ErrInvalid
		}
		seen[m] = true
	}
	h, tr := newSourceHTTP(limits)
	return &Service{scope: scope, secret: secret, allowed: append([]contract.SourceMapping(nil), allowed...), http: h, transport: tr}, nil
}
func (s *Service) CloseIdleConnections() {
	if s != nil && s.transport != nil {
		s.transport.CloseIdleConnections()
	}
}

// Reserve sends ONE explicitly requested service-authenticated reserve operation.
// It may create a remote immutable reserve even when its response is lost. No
// automatic retry/status/ack/renewal or local node/credential/permit is created.
// Returned row principal/session-binding are observed source DATA, not an own-
// PKCE result and not expected native identity. Config/clock/lifecycle authority
// of the calling registrar remains a separate unimplemented owner prerequisite.
func (s *Service) Reserve(ctx context.Context, request contract.SourceReserveRequest, now time.Time) (contract.SourceReserve, error) {
	if s == nil || s.http == nil || ctx == nil || ctx.Err() != nil || now.IsZero() {
		return contract.SourceReserve{}, ErrInvalid
	}
	if _, err := contract.ServiceReserveDescriptorBytes(request, s.scope.ServiceID); err != nil {
		return contract.SourceReserve{}, ErrInvalid
	}
	allowed := false
	for _, m := range s.allowed {
		if m == request.Mapping {
			allowed = true
		}
	}
	if !allowed {
		return contract.SourceReserve{}, ErrInvalid
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > 16384 {
		return contract.SourceReserve{}, ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.scope.SourceOrigin+"/api/mnet-enrollment-service-v1/reserve-evidence-v2", bytes.NewReader(raw))
	if err != nil {
		return contract.SourceReserve{}, ErrInvalid
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+s.scope.KeyID+"."+s.secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	req.Header.Set("Pragma", "no-cache")
	started := time.Now()
	response, err := s.http.Do(req)
	if err != nil {
		return contract.SourceReserve{}, ErrUnavailable
	}
	defer response.Body.Close()
	body, err := readSourceJSON(response)
	if err != nil {
		return contract.SourceReserve{}, ErrUnavailable
	}
	data, err := contract.DecodeServiceReserve(body, request, s.scope.PrincipalAuthority, s.scope.ServiceID, now.Add(time.Since(started)))
	if err != nil {
		return contract.SourceReserve{}, ErrUnavailable
	}
	return data, nil
}
