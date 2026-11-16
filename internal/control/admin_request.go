package control

import (
	"mime"
	"net/http"
)

// requireAdminJSON guards login and all cookie-authenticated mutations.
// Browser clients must stay on the configured public origin; scripts may
// omit browser headers but must still use JSON and a valid session.
func (s *Server) requireAdminJSON(w http.ResponseWriter, r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, "application/json required")
		return false
	}
	origin, site := r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site")
	if (origin != "" && origin != s.publicBase) || (site != "" && site != "same-origin" && site != "none") {
		writeErr(w, http.StatusForbidden, "same-origin request required")
		return false
	}
	return true
}
