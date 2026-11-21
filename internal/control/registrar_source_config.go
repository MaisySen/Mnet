package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"time"
	"unicode/utf8"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	source "fintex.au/mnet/internal/enrollmentsource"
)

const registrarSourceMaxBytes = 65536

var errRegistrarSource = errors.New("inactive registrar source configuration unavailable")
var errRegistrarSourceOff = errors.New("registrar sources not installed; inactive source workflow is off")

// Private explicit experimental schema. No defaults, legacy conversion,
// environment loader or installer. Secret is a separately provisioned service
// bearer, never node/native credential material or a source response field.
type registrarSourceConfig struct {
	Version       int                    `json:"version"`
	Revision      string                 `json:"revision"`
	MaxConcurrent int                    `json:"max_concurrent"`
	MaxPerSource  int                    `json:"max_per_source"`
	MaxAttempts   int                    `json:"max_attempts"`
	Sources       []registrarSourceEntry `json:"sources"`
}
type registrarSourceEntry struct {
	ID                 string                   `json:"id"`
	SourceOrigin       string                   `json:"source_origin"`
	PrincipalAuthority string                   `json:"principal_authority"`
	KeyID              string                   `json:"key_id"`
	ServiceID          string                   `json:"service_id"`
	IssuerLabel        string                   `json:"issuer_label"`
	RegistrarOrigin    string                   `json:"registrar_origin"`
	Audience           string                   `json:"audience"`
	Secret             string                   `json:"secret"`
	TimeoutMS          int                      `json:"timeout_ms"`
	MaxConnections     int                      `json:"max_connections"`
	Allowed            []contract.SourceMapping `json:"allowed"`
}

func (registrarSourceConfig) String() string   { return "[private inactive registrar source config]" }
func (registrarSourceConfig) GoString() string { return "[private inactive registrar source config]" }
func (registrarSourceEntry) String() string    { return "[private inactive registrar source entry]" }
func (registrarSourceEntry) GoString() string  { return "[private inactive registrar source entry]" }
func parseRegistrarSources(raw []byte) ([]source.PoolSource, source.PoolLimits, error) {
	var empty source.PoolLimits
	if len(raw) == 0 || len(raw) > registrarSourceMaxBytes || !utf8.Valid(raw) {
		return nil, empty, errRegistrarSource
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if !registrarConfigShape(decoder, reflect.TypeFor[registrarSourceConfig](), 0) {
		return nil, empty, errRegistrarSource
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, empty, errRegistrarSource
	}
	var c registrarSourceConfig
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || len(c.Revision) < 1 || len(c.Revision) > 128 || len(c.Sources) == 0 || len(c.Sources) > 32 {
		return nil, empty, errRegistrarSource
	}
	for _, r := range c.Revision {
		if r < 33 || r > 126 {
			return nil, empty, errRegistrarSource
		}
	}
	out := make([]source.PoolSource, 0, len(c.Sources))
	for _, entry := range c.Sources {
		if entry.TimeoutMS < 1 || entry.TimeoutMS > 60000 {
			return nil, empty, errRegistrarSource
		}
		out = append(out, source.PoolSource{ID: entry.ID, Scope: source.ServiceScope{SourceOrigin: entry.SourceOrigin, PrincipalAuthority: entry.PrincipalAuthority, KeyID: entry.KeyID, ServiceID: entry.ServiceID, Issuer: entry.IssuerLabel, RegistrarOrigin: entry.RegistrarOrigin, Audience: entry.Audience}, Secret: entry.Secret, Allowed: entry.Allowed, Transport: source.Limits{Timeout: time.Duration(entry.TimeoutMS) * time.Millisecond, MaxConnections: entry.MaxConnections}})
	}
	limits := source.PoolLimits{MaxSources: 32, MaxConcurrent: c.MaxConcurrent, MaxPerSource: c.MaxPerSource, MaxAttempts: c.MaxAttempts}
	// NewPool fully validates every service/scope/secret/map and all budget
	// relationships without HTTP. The temporary validator is closed, never used.
	validated, err := source.NewPool(out, limits)
	if err != nil {
		return nil, empty, errRegistrarSource
	}
	validated.Close()
	return out, limits, nil
}
func registrarConfigShape(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 8 {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fields[field.Tag.Get("json")] = field.Type
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
			if !registrarConfigShape(d, field, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim('}') && len(seen) == len(fields)
	case reflect.Slice:
		token, err := d.Token()
		if err != nil || token != json.Delim('[') {
			return false
		}
		count := 0
		for d.More() {
			count++
			if count > 32 || !registrarConfigShape(d, t.Elem(), depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim(']')
	case reflect.String:
		token, err := d.Token()
		_, ok := token.(string)
		return err == nil && ok
	case reflect.Int:
		token, err := d.Token()
		number, ok := token.(json.Number)
		if err != nil || !ok {
			return false
		}
		_, err = number.Int64()
		return err == nil
	}
	return false
}
