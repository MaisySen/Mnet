package enrollmentcontract

// Strict experimental v4 wire DATA only. Parsing authenticates no channel,
// expected identity, key possession, eligible lease, durable outcome or grant.
import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"unicode/utf8"
)

const ProofWireMaxBytes = 16384

// Versioned envelopes are a NEW inactive transport proposal. No v2/v3 fallback
// or caller-controlled reference resolver. Response/confirmation carry no binding.
type ChallengeWire struct {
	Version   int       `json:"version"`
	Challenge Challenge `json:"challenge"`
}
type ResponseWire struct {
	Version     int    `json:"version"`
	ChallengeID string `json:"challenge_id"`
	ResponseMAC string `json:"response_mac"`
}
type ConfirmationWire struct {
	Version         int    `json:"version"`
	ChallengeID     string `json:"challenge_id"`
	ConfirmationMAC string `json:"confirmation_mac"`
}

func (ChallengeWire) String() string      { return "[private untrusted v4 challenge wire DATA]" }
func (ChallengeWire) GoString() string    { return "[private untrusted v4 challenge wire DATA]" }
func (ResponseWire) String() string       { return "[private untrusted v4 response wire DATA]" }
func (ResponseWire) GoString() string     { return "[private untrusted v4 response wire DATA]" }
func (ConfirmationWire) String() string   { return "[private untrusted v4 confirmation wire DATA]" }
func (ConfirmationWire) GoString() string { return "[private untrusted v4 confirmation wire DATA]" }
func DecodeChallengeWire(raw []byte) (Challenge, error) {
	var w ChallengeWire
	if !proofWireExact(raw, &w) || w.Version != 4 {
		return Challenge{}, ErrInvalid
	}
	if _, err := Encode(w.Challenge); err != nil {
		return Challenge{}, ErrInvalid
	}
	return w.Challenge, nil
}
func DecodeResponseWire(raw []byte) (ResponseWire, error) {
	var w ResponseWire
	if !proofWireExact(raw, &w) || w.Version != 4 {
		return ResponseWire{}, ErrInvalid
	}
	if _, err := digest(w.ChallengeID); err != nil {
		return ResponseWire{}, ErrInvalid
	}
	if _, err := digest(w.ResponseMAC); err != nil {
		return ResponseWire{}, ErrInvalid
	}
	return w, nil
}
func DecodeConfirmationWire(raw []byte) (ConfirmationWire, error) {
	var w ConfirmationWire
	if !proofWireExact(raw, &w) || w.Version != 4 {
		return ConfirmationWire{}, ErrInvalid
	}
	if _, err := digest(w.ChallengeID); err != nil {
		return ConfirmationWire{}, ErrInvalid
	}
	if _, err := digest(w.ConfirmationMAC); err != nil {
		return ConfirmationWire{}, ErrInvalid
	}
	return w, nil
}
func proofWireExact(raw []byte, out any) bool {
	if len(raw) == 0 || len(raw) > ProofWireMaxBytes || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !proofWireShape(d, reflect.TypeOf(out).Elem(), 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
func proofWireShape(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 4 {
		return false
	}
	if t.Kind() == reflect.Struct {
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
			if !proofWireShape(d, field, depth+1) {
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
		value, ok := token.(string)
		return ok && utf8.ValidString(value)
	case reflect.Int, reflect.Int64:
		number, ok := token.(json.Number)
		if !ok {
			return false
		}
		_, err := number.Int64()
		return err == nil
	}
	return false
}
