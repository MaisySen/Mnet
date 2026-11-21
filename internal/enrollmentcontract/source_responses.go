package enrollmentcontract

// Strict untrusted DATA decoding for the actual Multies v2 responses. No HTTP
// client, origin authentication, secret store, native callback, grant or permit.
import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fintex.au/mnet/internal/enrollmentidentity"
)

type SourceLaunch struct {
	Version          int    `json:"version"`
	Purpose          string `json:"purpose"`
	AttemptID        string `json:"attemptId"`
	DevicePublicKey  string `json:"devicePublicKey"`
	CredentialDigest string `json:"credentialDigest"`
	Challenge        string `json:"challenge"`
	State            string `json:"state"`
	RedirectURI      string `json:"redirectUri"`
	Platform         string `json:"platform"`
}
type SourceProvenance struct {
	PrincipalAuthority string `json:"principalAuthority"`
	PrincipalNamespace string `json:"principalNamespace"`
	PrincipalID        string `json:"principalId"`
	SessionBinding     string `json:"sessionBinding"`
}
type SourceMapping struct {
	WorkspaceID     string `json:"workspaceId"`
	Issuer          string `json:"issuer"`
	RegistrarOrigin string `json:"registrarOrigin"`
	Audience        string `json:"audience"`
	MappingRevision string `json:"mappingRevision"`
	NetworkID       string `json:"networkId"`
}
type SourceExchange struct {
	ResponseVersion    int              `json:"responseVersion"`
	Provenance         SourceProvenance `json:"provenance"`
	Launch             SourceLaunch     `json:"launch"`
	Ticket             string           `json:"ticket"`
	ID                 string           `json:"id"`
	ExpiresAt          string           `json:"expiresAt"`
	Mapping            SourceMapping    `json:"binding"`
	Purpose            string           `json:"purpose"`
	RegistrarConnected bool             `json:"registrarConnected"`
	Enrolled           bool             `json:"enrolled"`
	InputAuthorized    bool             `json:"inputAuthorized"`
}

func (SourceExchange) String() string   { return "[private untrusted enrollment exchange data]" }
func (SourceExchange) GoString() string { return "[private untrusted enrollment exchange data]" }

type SourceReserve struct {
	HandoffID                 string           `json:"handoffId"`
	ReserveID                 string           `json:"reserveId"`
	DescriptorDigest          string           `json:"descriptorDigest"`
	Deadline                  string           `json:"deadline"`
	Status                    string           `json:"status"`
	Enrolled                  bool             `json:"enrolled"`
	ResponseVersion           int              `json:"responseVersion"`
	Provenance                SourceProvenance `json:"provenance"`
	Launch                    SourceLaunch     `json:"launch"`
	Mapping                   SourceMapping    `json:"binding"`
	ServiceID                 string           `json:"serviceId"`
	CheckedAt                 string           `json:"checkedAt"`
	DescriptorEncodingVersion int              `json:"descriptorEncodingVersion"`
	AuthorityGranted          bool             `json:"authorityGranted"`
}

var sourceCallback = regexp.MustCompile(`^http://127\.0\.0\.1:([1-9][0-9]{3,4})/mnet/enrollment/callback$`)

func validSourceLaunch(l SourceLaunch) bool {
	if l.Version != 1 || l.Purpose != "mnet-enrollment" || (l.Platform != "linux" && l.Platform != "macos" && l.Platform != "windows") {
		return false
	}
	for _, s := range []string{l.AttemptID, l.DevicePublicKey, l.CredentialDigest, l.Challenge, l.State} {
		if _, err := digest(s); err != nil {
			return false
		}
	}
	match := sourceCallback.FindStringSubmatch(l.RedirectURI)
	if match == nil {
		return false
	}
	port, _ := strconv.Atoi(match[1])
	if port < 1024 || port > 65535 {
		return false
	}
	u, err := url.Parse(l.RedirectURI)
	return err == nil && u.String() == l.RedirectURI
}

var sourceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func sourcePrincipal(s string) bool {
	if !text(s) || len(s) > 256 {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func validSourceMapping(m SourceMapping) bool {
	if !enrollmentidentity.ValidIssuer(m.Issuer) || !enrollmentidentity.ValidOrigin(m.RegistrarOrigin) || !sourcePrincipal(m.Audience) {
		return false
	}
	for _, s := range []string{m.WorkspaceID, m.MappingRevision, m.NetworkID} {
		if !sourceID.MatchString(s) {
			return false
		}
	}
	return true
}
func validSourceProvenance(p SourceProvenance) bool {
	if !enrollmentidentity.ValidOrigin(p.PrincipalAuthority) || p.PrincipalNamespace != PrincipalNamespace || !sourcePrincipal(p.PrincipalID) {
		return false
	}
	_, err := digest(p.SessionBinding)
	return err == nil
}
func sourceDate(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil && t.UnixMilli() > 0 && t.UTC().Format("2006-01-02T15:04:05.000Z") == s
}

// DecodeSourceExchange checks response data against a locally saved launch and
// explicit independently supplied current AND saved allowed mappings. It does
// NOT establish that caller authenticated the origin or owns native saved state.
func DecodeSourceExchange(raw []byte, saved SourceLaunch, authority string, savedAllowed, currentAllowed []SourceMapping, now time.Time) (SourceExchange, error) {
	var r SourceExchange
	if !decodeSourceExact(raw, &r) || !validSourceLaunch(saved) || r.Launch != saved || !validSourceProvenance(r.Provenance) || r.Provenance.PrincipalAuthority != authority || !validSourceMapping(r.Mapping) || r.ResponseVersion != 2 || r.Purpose != "mnet-enrollment" || r.RegistrarConnected || r.Enrolled || r.InputAuthorized || now.IsZero() {
		return SourceExchange{}, ErrInvalid
	}
	for _, s := range []string{r.Ticket, r.ID} {
		if _, err := digest(s); err != nil {
			return SourceExchange{}, err
		}
	}
	expires, ok := sourceDate(r.ExpiresAt)
	if !ok || !expires.After(now) {
		return SourceExchange{}, ErrInvalid
	}
	allowed := func(list []SourceMapping) bool {
		for _, m := range list {
			if m == r.Mapping {
				return true
			}
		}
		return false
	}
	if !allowed(savedAllowed) || !allowed(currentAllowed) {
		return SourceExchange{}, ErrInvalid
	}
	return r, nil
}

// DecodeSourceReserve compares against own-PKCE result, exact service/reserve/
// descriptor selected BEFORE the request, and explicit current saved mapping.
// It returns DATA only, not a fresh-authority permit or trusted clock.
func DecodeSourceReserve(raw []byte, own SourceExchange, serviceID, reserveID, descriptorDigest string, current SourceMapping, now time.Time) (SourceReserve, error) {
	var r SourceReserve
	if !decodeSourceExact(raw, &r) || r.ResponseVersion != 2 || r.DescriptorEncodingVersion != 1 || r.Status != "reserved" || r.Enrolled || r.AuthorityGranted || r.Provenance != own.Provenance || r.Launch != own.Launch || r.Mapping != own.Mapping || r.Mapping != current || r.HandoffID != own.ID || r.ServiceID != serviceID || !sourceID.MatchString(serviceID) || r.ReserveID != reserveID || r.DescriptorDigest != descriptorDigest || now.IsZero() {
		return SourceReserve{}, ErrInvalid
	}
	computed, err := SourceDescriptorDigest(own, serviceID, reserveID)
	if err != nil || computed != descriptorDigest {
		return SourceReserve{}, ErrInvalid
	}
	if !validSourceLaunch(r.Launch) || !validSourceMapping(r.Mapping) || !validSourceProvenance(r.Provenance) {
		return SourceReserve{}, ErrInvalid
	}
	for _, s := range []string{r.HandoffID, r.ReserveID, r.DescriptorDigest} {
		if _, err := digest(s); err != nil {
			return SourceReserve{}, err
		}
	}
	checked, ok := sourceDate(r.CheckedAt)
	if !ok || checked.After(now) {
		return SourceReserve{}, ErrInvalid
	}
	deadline, ok := sourceDate(r.Deadline)
	if !ok || !deadline.After(now) || !deadline.After(checked) {
		return SourceReserve{}, ErrInvalid
	}
	return r, nil
}

func decodeSourceExact(raw []byte, out any) bool {
	if len(raw) == 0 || len(raw) > 65536 || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if !sourceShape(d, reflect.TypeOf(out).Elem(), 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
func sourceShape(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 8 {
		return false
	}
	if t.Kind() == reflect.Struct {
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fields[f.Tag.Get("json")] = f.Type
		}
		seen := map[string]bool{}
		for d.More() {
			token, err = d.Token()
			name, ok := token.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			field, ok := fields[name]
			if !ok {
				return false
			}
			seen[name] = true
			if !sourceShape(d, field, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim('}') && len(seen) == len(fields)
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	switch t.Kind() {
	case reflect.String:
		s, ok := token.(string)
		return ok && !strings.ContainsRune(s, utf8.RuneError)
	case reflect.Bool:
		_, ok := token.(bool)
		return ok
	case reflect.Int:
		n, ok := token.(float64)
		return ok && (n == 1 || n == 2)
	}
	return false
}
