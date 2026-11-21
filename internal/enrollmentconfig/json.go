package enrollmentconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"unicode/utf8"
)

const MaxBytes = 256 * 1024

// Parse validates an explicit operator handoff document, not active setup.
// Exact field names and duplicates are checked before encoding/json can apply
// its case-insensitive or last-value-wins behavior. It creates no authority.
func Parse(raw []byte) (*Mapping, error) {
	if len(raw) > MaxBytes || !utf8.Valid(raw) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !exactValue(d, reflect.TypeFor[Config](), 0) {
		return nil, ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return nil, ErrInvalid
	}
	var c Config
	if e := json.Unmarshal(raw, &c); e != nil {
		return nil, ErrInvalid
	}
	return Validate(c)
}
func exactValue(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 8 {
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
			if !exactValue(d, ft, depth+1) {
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
			if n >= 1024 || !exactValue(d, t.Elem(), depth+1) {
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
