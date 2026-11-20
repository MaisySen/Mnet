package multies

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
)

// validateBridgeJSON rejects duplicate fields and case-insensitive struct-field
// aliases before encoding/json applies its permissive last-value-wins rules.
// The input is already size bounded by call. No arbitrary maps are accepted in
// this protocol, and JSON-unmarshaling scalar types (time.Time) remain scalars.
func validateBridgeJSON(raw []byte, out any) bool {
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if !bridgeJSONValue(d, t.Elem(), 0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

var bridgeJSONUnmarshaler = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

func bridgeJSONValue(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 16 {
		return false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if reflect.PointerTo(t).Implements(bridgeJSONUnmarshaler) {
		token, err := d.Token()
		_, ok := token.(string)
		return err == nil && ok
	}
	switch t.Kind() {
	case reflect.Struct:
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = f.Type
		}
		seen := map[string]bool{}
		for d.More() {
			token, err = d.Token()
			name, ok := token.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			field, exists := fields[name]
			if !exists {
				return false
			}
			seen[name] = true
			if !bridgeJSONValue(d, field, depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim('}')
	case reflect.Slice:
		token, err := d.Token()
		if err != nil {
			return false
		}
		if token == nil {
			return true
		}
		if token != json.Delim('[') {
			return false
		}
		for d.More() {
			if !bridgeJSONValue(d, t.Elem(), depth+1) {
				return false
			}
		}
		token, err = d.Token()
		return err == nil && token == json.Delim(']')
	case reflect.String:
		token, err := d.Token()
		_, ok := token.(string)
		return err == nil && ok
	case reflect.Bool:
		token, err := d.Token()
		_, ok := token.(bool)
		return err == nil && ok
	case reflect.Int, reflect.Int64:
		token, err := d.Token()
		_, ok := token.(float64)
		return err == nil && ok
	default:
		return false
	}
}
