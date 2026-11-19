package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type localUpRequest struct {
	Server          string
	Network         string
	Name            string
	AuthKey         string
	AdvertiseExit   bool
	AdvertiseRoutes []string
	UseExitNode     string
	ResetRouting    bool
}

// Up changes saved identity/configuration and starts asynchronous work. Require
// one exact object so a typo, duplicate key or trailing value cannot silently
// choose a target or turn a malformed request into saved-state startup. Empty
// objects intentionally retain the CLI's ordinary saved-state Up semantics.
func decodeLocalUp(dec *json.Decoder) (localUpRequest, error) {
	var request localUpRequest
	invalid := fmt.Errorf("invalid local Up request")
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return request, invalid
	}
	seen := make(map[string]bool)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return request, invalid
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return request, invalid
		}
		seen[key] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return request, invalid
		}
		var target any
		switch key {
		case "server":
			target = &request.Server
		case "network":
			target = &request.Network
		case "name":
			target = &request.Name
		case "auth_key":
			target = &request.AuthKey
		case "advertise_exit_node":
			target = &request.AdvertiseExit
		case "exit_node":
			target = &request.UseExitNode
		case "reset_routing":
			target = &request.ResetRouting
		case "advertise_routes":
			var values []json.RawMessage
			if err := json.Unmarshal(raw, &values); err != nil {
				return request, invalid
			}
			for _, value := range values {
				var route string
				if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &route) != nil {
					return request, invalid
				}
				request.AdvertiseRoutes = append(request.AdvertiseRoutes, route)
			}
			continue
		default:
			return request, invalid
		}
		if json.Unmarshal(raw, target) != nil {
			return request, invalid
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') {
		return request, invalid
	}
	if _, err = dec.Token(); err != io.EOF {
		return request, invalid
	}
	return request, nil
}
