package control

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

// Server is the control plane HTTP server.
type Server struct {
	store      *Store
	hub        *relay.Hub
	relayURL   string // public WS URL advertised to clients
	upgrader   websocket.Upgrader
	sessions   map[string]time.Time // admin session token -> expiry
	sessionsMu sync.Mutex

	oidc         *OIDCProvider // nil: manual approval only
	oidcMu       sync.Mutex
	oidcStates   map[string]oidcState // login state -> waiting node
	oidcStatesMu sync.Mutex

	adminPassword string
	ownPassword   bool // true when generated, so we log it once

	loginLim  *ipLimiters // admin login brute-force protection
	joinLim   *ipLimiters // join flood protection
	auditPath string       // append-only audit log, <store>.audit.jsonl
	auditMu   sync.Mutex
	metrics   *metrics
}

// NewServer builds a server. adminPassword may be empty, in which case a
// random one is generated and returned for the caller to print.
func NewServer(store *Store, hub *relay.Hub, publicBase, adminPassword string) (*Server, string, error) {
	generated := ""
	if adminPassword == "" {
		b := make([]byte, 12)
		rand.Read(b)
		generated = base64.RawURLEncoding.EncodeToString(b)
		adminPassword = generated
	}
	s := &Server{
		store:    store,
		hub:      hub,
		relayURL: strings.TrimRight(publicBase, "/") + "/api/relay/ws",
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		sessions:      map[string]time.Time{},
		oidcStates:    map[string]oidcState{},
		adminPassword: adminPassword,
		loginLim:      newIPLimiters(10, 5),
		joinLim:       newIPLimiters(60, 20),
		auditPath:     store.Path() + ".audit.jsonl",
		metrics:       &metrics{StartedAt: time.Now().UTC()},
	}
	onLimited := func() { s.metrics.bump(&s.metrics.RateLimited) }
	s.loginLim.onLimit = onLimited
	s.joinLim.onLimit = onLimited
	return s, generated, nil
}

// RelayURL is what clients dial for relayed traffic.
func (s *Server) RelayURL() string { return s.relayURL }

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/", s.handleLanding)
	mux.HandleFunc("/admin", s.handleDashboardPage)
	mux.HandleFunc("/join/approve", s.handleApprovePage)
	mux.HandleFunc("/oidc/start", s.handleOIDCStart)
	mux.HandleFunc("/oidc/callback", s.handleOIDCCallback)
	mux.HandleFunc("/api/join", s.joinLim.limit(s.handleJoin))
	mux.HandleFunc("/api/join/status", s.handleJoinStatus)
	mux.HandleFunc("/api/map", s.handleMap)
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/relay/ws", s.handleRelayWS)
	mux.HandleFunc("/admin/api/login", s.loginLim.limit(s.handleAdminLogin))
	mux.HandleFunc("/admin/api/state", s.adminOnly(s.handleAdminState))
	mux.HandleFunc("/admin/api/networks", s.adminOnly(s.handleAdminNetworks))
	mux.HandleFunc("/admin/api/authkeys", s.adminOnly(s.handleAdminAuthKeys))
	mux.HandleFunc("/admin/api/authkeys/delete", s.adminOnly(s.handleAdminAuthKeyDelete))
	mux.HandleFunc("/admin/api/nodes/approve", s.adminOnly(s.handleAdminNodeApprove))
	mux.HandleFunc("/admin/api/nodes/deny", s.adminOnly(s.handleAdminNodeDeny))
	mux.HandleFunc("/admin/api/nodes/delete", s.adminOnly(s.handleAdminNodeDelete))
	mux.HandleFunc("/admin/api/nodes/expire", s.adminOnly(s.handleAdminNodeExpire))
	mux.HandleFunc("/admin/api/acl", s.adminOnly(s.handleAdminACL))
	mux.HandleFunc("/admin/api/metrics", s.adminOnly(s.handleAdminMetrics))
	return securityHeaders(mux)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, netmap.APIError{Error: msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json: "+err.Error())
		return false
	}
	return true
}

func (s *Server) nodeAuth(r *http.Request) (*netmap.Node, bool) {
	id := r.Header.Get("X-Node-ID")
	tok := r.Header.Get("X-Node-Token")
	if id == "" || tok == "" {
		id = r.URL.Query().Get("node_id")
		tok = r.URL.Query().Get("token")
	}
	n, err := s.store.AuthorizeNode(id, tok)
	if err != nil || n.KeyExpired() {
		return nil, false
	}
	return n, true
}

// Node API

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req netmap.JoinRequest
	if !readJSON(w, r, &req) {
		return
	}
	resp, err := s.store.Join(req)
	if err != nil {
		s.audit(r, "join", req.Name, req.Network+": "+err.Error(), false)
		s.metrics.bump(&s.metrics.JoinsFailed)
		switch err {
		case ErrNotFound:
			writeErr(w, http.StatusNotFound, "network not found")
		case ErrBadAuthKey:
			writeErr(w, http.StatusForbidden, err.Error())
		case ErrBadName:
			writeErr(w, http.StatusBadRequest, "names use lowercase letters, numbers and dashes")
		default:
			writeErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	s.audit(r, "join", req.Name, req.Network, true)
	s.metrics.bump(&s.metrics.JoinsOK)
	if !resp.Approved {
		if s.oidcProvider() != nil {
			// SSO login replaces manual approval.
			resp.ApprovalURL = publicURL(r, "/oidc/start?node_id="+resp.NodeID+"&token="+resp.NodeToken)
		} else {
			resp.ApprovalURL = publicURL(r, "/join/approve?node_id="+resp.NodeID)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleJoinStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.JoinStatus(r.URL.Query().Get("node_id"), r.URL.Query().Get("token"))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unknown node or token")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	node, ok := s.nodeAuth(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unknown node or token")
		return
	}
	if !node.Approved {
		writeErr(w, http.StatusForbidden, "node not approved yet")
		return
	}
	s.store.Touch(node.ID)
	var since int64
	if v := r.URL.Query().Get("since"); v != "" {
		fmt_Sscanf(v, &since)
	}
	if wait := r.URL.Query().Get("wait"); wait == "1" {
		s.store.WaitForChange(node.NetworkID, since, 50*time.Second)
	}
	m, err := s.store.Map(node.ID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "node gone")
		return
	}
	m.RelayURL = s.relayURL + "?node_id=" + node.ID + "&token=" + r.URL.Query().Get("token")
	if tok := r.Header.Get("X-Node-Token"); tok != "" {
		m.RelayURL = s.relayURL + "?node_id=" + node.ID + "&token=" + tok
	}
	writeJSON(w, http.StatusOK, m)
}

// fmt_Sscanf avoids pulling fmt into the hot path for one integer.
func fmt_Sscanf(s string, v *int64) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	*v = n
}

func (s *Server) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	node, ok := s.nodeAuth(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unknown node or token")
		return
	}
	var up netmap.EndpointsUpdate
	if !readJSON(w, r, &up) {
		return
	}
	if err := s.store.SetEndpoints(node.ID, up.Endpoints); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRelayWS(w http.ResponseWriter, r *http.Request) {
	node, ok := s.nodeAuth(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unknown node or token")
		return
	}
	if !node.Approved {
		writeErr(w, http.StatusForbidden, "node not approved yet")
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("relay upgrade: %v", err)
		return
	}
	s.store.Touch(node.ID)
	s.hub.Attach(node.ID, node.PubKey, ws, nil)
}

// Admin auth

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.adminPassword)) != 1 {
		s.audit(r, "admin_login", "admin", "", false)
		s.metrics.bump(&s.metrics.LoginsFailed)
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	s.audit(r, "admin_login", "admin", "", true)
	s.metrics.bump(&s.metrics.LoginsOK)
	b := make([]byte, 24)
	rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	s.sessionsMu.Lock()
	s.sessions[tok] = time.Now().Add(24 * time.Hour)
	s.sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "mnet_admin",
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("mnet_admin")
		ok := false
		if err == nil {
			s.sessionsMu.Lock()
			exp, found := s.sessions[c.Value]
			ok = found && time.Now().Before(exp)
			s.sessionsMu.Unlock()
		}
		if !ok {
			writeErr(w, http.StatusUnauthorized, "sign in first")
			return
		}
		next(w, r)
	}
}

// Admin API

func (s *Server) handleAdminState(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()
	writeJSON(w, http.StatusOK, struct {
		*Snapshot
		Online map[string]bool `json:"online"`
	}{snap, s.hub.OnlineMap()})
}

func (s *Server) handleAdminNetworks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	n, err := s.store.CreateNetwork(body.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleAdminAuthKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var body struct {
		NetworkID string `json:"network_id"`
		Note      string `json:"note"`
		Reusable  bool   `json:"reusable"`
		TTLHours  int    `json:"ttl_hours"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	k, err := s.store.CreateAuthKey(body.NetworkID, body.Note, body.Reusable, time.Duration(body.TTLHours)*time.Hour)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "authkey_create", "admin", body.NetworkID, true)
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleAdminAuthKeyDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.store.DeleteAuthKey(body.ID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.audit(r, "authkey_delete", "admin", body.ID, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminNodeApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID string `json:"node_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.store.ApproveNode(body.NodeID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.audit(r, "node_approve", "admin", body.NodeID, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminNodeDeny(w http.ResponseWriter, r *http.Request) {
	// Deny removes the pending node so a retry gets a fresh slate.
	s.handleAdminNodeDelete(w, r)
}

// handleAdminNodeExpire forces a node's key to expire immediately.
func (s *Server) handleAdminNodeExpire(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID string `json:"node_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.store.ExpireNode(body.NodeID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.audit(r, "node_expire", "admin", body.NodeID, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminNodeDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID string `json:"node_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.store.DeleteNode(body.NodeID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.audit(r, "node_delete", "admin", body.NodeID, true)
	s.hub.Detach(body.NodeID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminMetrics reports operational counters and fleet size.
func (s *Server) handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Snapshot()
	approved, pending := 0, 0
	for _, n := range snap.Nodes {
		if n.Approved {
			approved++
		} else {
			pending++
		}
	}
	s.metrics.mu.Lock()
	m := metrics{
		StartedAt:    s.metrics.StartedAt,
		JoinsOK:      s.metrics.JoinsOK,
		JoinsFailed:  s.metrics.JoinsFailed,
		LoginsOK:     s.metrics.LoginsOK,
		LoginsFailed: s.metrics.LoginsFailed,
		RateLimited:  s.metrics.RateLimited,
	}
	s.metrics.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"started_at":       m.StartedAt,
		"uptime_seconds":   int64(time.Since(m.StartedAt).Seconds()),
		"joins_ok":         m.JoinsOK,
		"joins_failed":     m.JoinsFailed,
		"logins_ok":        m.LoginsOK,
		"logins_failed":    m.LoginsFailed,
		"rate_limited":     m.RateLimited,
		"networks":         len(snap.Networks),
		"nodes_approved":   approved,
		"nodes_pending":    pending,
		"nodes_online_now": len(s.hub.OnlineMap()),
	})
}
