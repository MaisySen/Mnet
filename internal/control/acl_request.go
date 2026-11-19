package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"fintex.au/mnet/internal/netmap"
)

type aclRequest struct {
	NetworkID string
	Rules     []netmap.ACLRule
}

// Omitting or misspelling rules must not silently clear a restrictive policy.
// Explicit null retains the existing API clear operation, like an empty array.
func decodeACLRequest(dec *json.Decoder) (aclRequest, error) {
	var request aclRequest
	invalid := errors.New("invalid ACL request")
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return request, invalid
	}
	seen := map[string]bool{}
	for dec.More() {
		token, err := dec.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return request, invalid
		}
		seen[key] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return request, invalid
		}
		switch key {
		case "network_id":
			if json.Unmarshal(raw, &request.NetworkID) != nil || request.NetworkID == "" {
				return request, invalid
			}
		case "rules":
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			var rules []json.RawMessage
			if json.Unmarshal(raw, &rules) != nil {
				return request, invalid
			}
			for _, rawRule := range rules {
				rule, err := decodeACLRule(json.NewDecoder(bytes.NewReader(rawRule)))
				if err != nil {
					return request, invalid
				}
				request.Rules = append(request.Rules, rule)
			}
		default:
			return request, invalid
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') || !seen["network_id"] || !seen["rules"] {
		return request, invalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return request, invalid
	}
	return request, nil
}

func decodeACLRule(dec *json.Decoder) (netmap.ACLRule, error) {
	var rule netmap.ACLRule
	invalid := errors.New("invalid ACL rule")
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return rule, invalid
	}
	seen := map[string]bool{}
	for dec.More() {
		token, err := dec.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return rule, invalid
		}
		seen[key] = true
		var target *string
		switch key {
		case "src":
			target = &rule.Src
		case "dst":
			target = &rule.Dst
		case "comment":
			target = &rule.Comment
		default:
			return rule, invalid
		}
		var raw json.RawMessage
		if dec.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, target) != nil {
			return rule, invalid
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') || !seen["src"] || !seen["dst"] {
		return rule, invalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return rule, invalid
	}
	return rule, nil
}
