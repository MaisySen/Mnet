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
	var body struct {
		NetworkID string `json:"network_id"`
		Issuer    string `json:"issuer"`
		Subject   string `json:"subject"`
		Allow     *bool  `json:"allow"`
	}
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid membership request")
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF || body.Allow == nil || body.NetworkID == "" || body.Issuer == "" || body.Subject == "" {
		writeErr(w, http.StatusBadRequest, "network_id, issuer, subject and explicit allow required")
		return
	}
	event := "membership_revoke"
	if *body.Allow {
		event = "membership_grant"
	}
	err := s.store.SetNetworkMember(body.NetworkID, body.Issuer, body.Subject, *body.Allow)
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
