package control

import (
	"net/http"
	"strings"
	"time"
)

const maxAdminSessions = 1024

func (s *Server) adminCookie(w http.ResponseWriter, token string, expires time.Time, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: "mnet_admin", Value: token, Path: "/", HttpOnly: true,
		Secure: strings.HasPrefix(s.publicBase, "https://"), SameSite: http.SameSiteLaxMode,
		Expires: expires, MaxAge: maxAge,
	})
}

// Logging out invalidates the server-side session, not just the browser cookie.
// Like every admin mutation this route is POST, JSON and origin guarded.
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("mnet_admin")
	if err == nil {
		s.sessionsMu.Lock()
		delete(s.sessions, c.Value)
		s.sessionsMu.Unlock()
	}
	s.adminCookie(w, "", time.Unix(1, 0), -1)
	s.audit(r, "admin_logout", "admin", "", true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
