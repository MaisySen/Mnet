package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fintex.au/mnet/internal/localapi"
	"fintex.au/mnet/internal/version"
)

// Status is the CLI's view of the daemon.
type Status struct {
	DNSPackets  map[string]bool `json:"dns_packets,omitempty"`
	DNSFailure  string          `json:"dns_failure,omitempty"` // fixed non-secret failure category
	DNSError    string          `json:"dns_error,omitempty"`
	DNSReady    bool            `json:"dns_ready"`           // both listeners active, not end-to-end traffic proof
	Interface   string          `json:"interface,omitempty"` // actual managed kernel device, not requested alias
	Status      string          `json:"status"`              // down, preparing, joining, waiting-approval, up, key-expired
	Version     string          `json:"version"`
	Server      string          `json:"server,omitempty"`
	Network     string          `json:"network,omitempty"`
	NodeName    string          `json:"node_name,omitempty"`
	IPv4        string          `json:"ipv4,omitempty"`
	PublicKey   string          `json:"public_key,omitempty"`
	ApprovalURL string          `json:"approval_url,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
	Peers       []PeerStatus    `json:"peers,omitempty"`
}

// Status renders current state.
func (d *Daemon) Status() *Status {
	d.mu.Lock()
	st := &Status{
		Status:      d.status,
		Interface:   d.interfaceName,
		Version:     version.Version,
		Server:      d.state.Server,
		Network:     d.state.Network,
		NodeName:    d.state.NodeName,
		PublicKey:   d.state.PublicKey,
		ApprovalURL: d.approvalURL,
		LastError:   d.lastErr,
		DNSError:    d.dnsErr,
		DNSFailure:  dnsFailureCategory(d.dnsErr),
	}
	if d.lastMap != nil {
		st.IPv4 = d.lastMap.Self.IPv4
		st.Network = d.lastMap.Network.Name
	}
	d.mu.Unlock()
	d.applyMu.Lock()
	st.DNSReady = d.dns != nil && d.dns.Ready()
	if d.localDNS != nil {
		st.DNSPackets = d.localDNS.Diagnostics()
	}
	d.applyMu.Unlock()
	st.Peers = d.PeerStatuses()
	return st
}

// Only fixed categories may cross public diagnostic boundaries.
func dnsFailureCategory(detail string) string {
	switch {
	case detail == "":
		return "none"
	case strings.Contains(detail, "permission denied") || strings.Contains(detail, "operation not permitted"):
		return "permission"
	case strings.Contains(detail, "cannot assign requested address"):
		return "address"
	case strings.Contains(detail, "address already in use"):
		return "occupied"
	default:
		return "other"
	}
}

// SocketPath is where the local API listens.
func SocketPath(stateDir string) (network, addr string) {
	if runtime.GOOS == "windows" {
		return "tcp", "127.0.0.1:47373"
	}
	return "unix", filepath.Join(stateDir, "mnetd.sock")
}

// ServeLocalAPI runs the CLI-facing API until the daemon stops.
func (d *Daemon) ServeLocalAPI() error {
	return d.ServeLocalAPIContext(context.Background(), nil)
}

// ServeLocalAPIContext reports readiness only after binding and closes the
// listener/HTTP handlers when ctx ends. ready receives setup errors too.
func (d *Daemon) ServeLocalAPIContext(ctx context.Context, ready chan<- error) error {
	notify := func(err error) {
		if ready != nil {
			ready <- err
		}
	}
	token, err := localapi.EnsureToken(d.cfg.StateDir)
	if err != nil {
		notify(err)
		return err
	}
	network, addr := SocketPath(d.cfg.StateDir)
	if network == "unix" {
		os.Remove(addr)
		if err := os.MkdirAll(filepath.Dir(addr), 0o700); err != nil {
			notify(err)
			return err
		}
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		err = fmt.Errorf("local api listen: %w", err)
		notify(err)
		return err
	}
	if network == "unix" {
		os.Chmod(addr, 0o600)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/localapi/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d.Status())
	})
	mux.HandleFunc("/localapi/session-peers", d.serveSessionPeers)
	mux.HandleFunc("/localapi/up", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Server          string   `json:"server"`
			Network         string   `json:"network"`
			Name            string   `json:"name"`
			AuthKey         string   `json:"auth_key"`
			AdvertiseExit   bool     `json:"advertise_exit_node"`
			AdvertiseRoutes []string `json:"advertise_routes"`
			UseExitNode     string   `json:"exit_node"`
			ResetRouting    bool     `json:"reset_routing"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid request JSON", http.StatusBadRequest)
			return
		}
		if req.ResetRouting && (req.AdvertiseExit || len(req.AdvertiseRoutes) > 0 || req.UseExitNode != "") {
			http.Error(w, "reset_routing cannot be combined with routing roles", http.StatusBadRequest)
			return
		}
		go func() {
			if req.ResetRouting {
				if err := d.SetRouting(false, nil, ""); err != nil {
					d.setErr(fmt.Errorf("reset routing: %w", err))
					return
				}
			}
			if req.AdvertiseExit || len(req.AdvertiseRoutes) > 0 || req.UseExitNode != "" {
				if err := d.SetRouting(req.AdvertiseExit, req.AdvertiseRoutes, req.UseExitNode); err != nil {
					d.setErr(fmt.Errorf("routing: %w", err))
					return
				}
			}
			if err := d.Up(req.Server, req.Network, req.Name, req.AuthKey); err != nil {
				d.setErr(fmt.Errorf("up: %w", err))
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "starting"})
	})
	mux.HandleFunc("/localapi/down", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		d.Down()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "down"})
	})
	server := &http.Server{Handler: localapi.RequireToken(token, mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-stopped:
		}
	}()
	notify(nil)
	err = server.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
