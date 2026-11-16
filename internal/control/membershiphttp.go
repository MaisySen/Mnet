package control

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// handleAdminMembership is explicit operator management, not identity login or
// self-service enrollment. The supplied principal is the grant target, never
// authentication for the caller. adminOnly must wrap this handler.
func (s *Server) handleAdminMembership(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		network := r.URL.Query().Get("network_id")
		if network == "" {
			writeErr(w, 400, "network_id required")
			return
		}
		members, err := s.store.OperatorMemberships(network)
		if err != nil {
			writeErr(w, 404, "unknown network")
			return
		}
		writeJSON(w, 200, map[string]any{"memberships": members})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if !s.requireAdminJSON(w, r) {
		return
	}
	defer r.Body.Close()
	body, err := decodeMembershipRequest(json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "network_id, issuer, subject and explicit allow required")
		return
	}
	event := "membership_revoke"
	if *body.Allow {
		event = "membership_grant"
	}
	err = s.store.SetNetworkMember(body.NetworkID, body.Issuer, body.Subject, *body.Allow)
	s.audit(r, event, "admin", body.NetworkID, err == nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeErr(w, http.StatusNotFound, "unknown network")
		} else {
			writeErr(w, http.StatusInternalServerError, "membership could not be saved")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type membershipRequest struct {
	NetworkID string
	Issuer    string
	Subject   string
	Allow     *bool
}

// Principal mutations accept exact field names once each, not JSON's default
// last-key-wins or case-insensitive matching of struct fields.
func decodeMembershipRequest(dec *json.Decoder) (membershipRequest, error) {
	var body membershipRequest
	invalid := errors.New("invalid membership request")
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return body, invalid
	}
	seen := map[string]bool{}
	for dec.More() {
		token, err := dec.Token()
		field, ok := token.(string)
		if err != nil || !ok || seen[field] {
			return body, invalid
		}
		seen[field] = true
		var target any
		switch field {
		case "network_id":
			target = &body.NetworkID
		case "issuer":
			target = &body.Issuer
		case "subject":
			target = &body.Subject
		case "allow":
			target = &body.Allow
		default:
			return body, invalid
		}
		if err := dec.Decode(target); err != nil {
			return body, invalid
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') || dec.Decode(&struct{}{}) != io.EOF || body.Allow == nil || body.NetworkID == "" || body.Issuer == "" || body.Subject == "" {
		return body, invalid
	}
	return body, nil
}
