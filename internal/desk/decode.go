package desk

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Decode checks the entire bounded document, including duplicate keys, before
// interpreting geometry. A hand-edited file must not silently pick one of two
// conflicting widths/devices. The result still has no identity/authority.
func Decode(r io.Reader) (Layout, error) {
	data, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil {
		return Layout{}, errors.New("layout file unreadable")
	}
	if len(data) > 65536 {
		return Layout{}, errors.New("layout file exceeds64KiB")
	}
	if err = validateJSON(data); err != nil {
		return Layout{}, err
	}
	var l Layout
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&l); err != nil {
		return Layout{}, errors.New("layout schema invalid or unknown fields present")
	}
	return l, nil
}

func validateJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var check func(int) error
	check = func(depth int) error {
		if depth > 8 {
			return errors.New("layout nesting exceeds policy")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] || name != strings.ToLower(name) {
					return errors.New("duplicate or invalid layout key")
				}
				seen[name] = true
				if err = check(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := check(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid layout structure")
		}
		_, err = d.Token()
		return err
	}
	if err := check(0); err != nil {
		return errors.New("layout JSON invalid, duplicated or too deeply nested")
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("layout must contain exactly one document")
	}
	return nil
}
