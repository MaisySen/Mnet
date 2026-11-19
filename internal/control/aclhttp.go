package control

import (
	"encoding/json"
	"net/http"

	"fintex.au/mnet/internal/netmap"
)

// handleAdminACL reads (GET) or replaces (PUT) a network's ACL policy.
// GET takes ?network_id=, PUT takes {network_id, rules:[...]}. An empty
// rules list clears the policy back to allow-all.
func (s *Server) handleAdminACL(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id := r.URL.Query().Get("network_id")
		n := s.store.Network(id)
		if n == nil {
			writeErr(w, http.StatusNotFound, "unknown network")
			return
		}
		pol := n.ACL
		if pol == nil {
			pol = &netmap.ACLPolicy{}
		}
		writeJSON(w, http.StatusOK, pol)
	case http.MethodPut:
		defer r.Body.Close()
		body, err := decodeACLRequest(json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Invalid policy request. Use src, dst and optional comment only in each rule.")
			return
		}
		var pol *netmap.ACLPolicy
		if len(body.Rules) > 0 {
			pol = &netmap.ACLPolicy{Rules: body.Rules}
		}
		if err := s.store.SetACL(body.NetworkID, pol); err != nil {
			code := http.StatusBadRequest
			if err == ErrNotFound {
				code = http.StatusNotFound
			}
			writeErr(w, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET or PUT only")
	}
}
