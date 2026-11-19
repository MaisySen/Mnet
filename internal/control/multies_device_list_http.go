package control

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// MultiesDeviceListConfig is a dedicated loopback mTLS listener, not a route on
// the Mnet public control listener. No legacy control/auth key can reach it.
type MultiesDeviceListConfig struct {
	Enabled, Deployment, WorkspaceID, Origin, Listen string
	CertFile, KeyFile, ClientCAFile                  string
}

// NewMultiesDeviceListServer treats authenticated Multies as the authority for
// current person/session membership. It trusts only a dedicated client cert
// with this list-only, workspace-bound URI SAN, not forwarded session claims.
func NewMultiesDeviceListServer(store *Store, c MultiesDeviceListConfig) (*http.Server, error) {
	if c.Enabled == "" || c.Enabled == "off" {
		return nil, nil
	}
	invalid := errors.New("invalid Multies device list configuration")
	if store == nil || c.Enabled != "on" || c.Deployment != "staging" || !inventoryWorkspaceID.MatchString(c.WorkspaceID) {
		return nil, invalid
	}
	addr, _, err := net.SplitHostPort(c.Listen)
	if err != nil || !net.ParseIP(addr).IsLoopback() {
		return nil, invalid
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Scheme != "https" || c.Origin != "https://"+origin.Host ||
		!strings.Contains(origin.Hostname(), ".") || net.ParseIP(origin.Hostname()) != nil || strings.HasSuffix(origin.Hostname(), ".") {
		return nil, invalid
	}
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil || cert.Leaf == nil || cert.Leaf.VerifyHostname(origin.Hostname()) != nil {
		return nil, invalid
	}
	ca, err := os.ReadFile(c.ClientCAFile)
	if err != nil {
		return nil, invalid
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, invalid
	}
	const prefix = "/internal/multies/v1/workspaces/"
	path := prefix + c.WorkspaceID + "/devices"
	identity := "urn:multies:staging:mnet-device-list:" + c.WorkspaceID
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != origin.Host || r.URL.EscapedPath() != path || r.URL.RawQuery != "" || r.URL.ForceQuery {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.TLS == nil || r.TLS.ServerName != origin.Hostname() || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		peer := r.TLS.VerifiedChains[0][0]
		now := time.Now()
		if len(peer.URIs) != 1 || peer.URIs[0].String() != identity || now.Before(peer.NotBefore) || !now.Before(peer.NotAfter) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		store.mu.Lock()
		devices := store.listMultiesDevicesLocked(c.WorkspaceID)
		store.mu.Unlock()
		if r.Context().Err() != nil {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Devices []InventoryDevice `json:"devices"`
		}{Devices: devices})
	})
	srv := NewHTTPServer(c.Listen, handler)
	srv.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, SessionTicketsDisabled: true,
	}
	return srv, nil
}
