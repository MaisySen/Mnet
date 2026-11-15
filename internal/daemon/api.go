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
	"time"

	"fintex.au/mnet/internal/localapi"
	"fintex.au/mnet/internal/version"
)

// Status is the CLI's view of the daemon.
type Status struct {
	Status      string       `json:"status"` // down, joining, waiting-approval, up, key-expired
	Version     string       `json:"version"`
	Server      string       `json:"server,omitempty"`
	Network     string       `json:"network,omitempty"`
	NodeName    string       `json:"node_name,omitempty"`
	IPv4        string       `json:"ipv4,omitempty"`
	PublicKey   string       `json:"public_key,omitempty"`
	ApprovalURL string       `json:"approval_url,omitempty"`
	LastError   string       `json:"last_error,omitempty"`
	Peers       []PeerStatus `json:"peers,omitempty"`
}

// Status renders current state.
func (d *Daemon) Status() *Status {
	d.mu.Lock()
	st := &Status{
		Status:      d.status,
		Version:     version.Version,
		Server:      d.state.Server,
		Network:     d.state.Network,
		NodeName:    d.state.NodeName,
		PublicKey:   d.state.PublicKey,
		ApprovalURL: d.approvalURL,
		LastError:   d.lastErr,
	}
	if d.lastMap != nil {
		st.IPv4 = d.lastMap.Self.IPv4
		st.Network = d.lastMap.Network.Name
	}
	d.mu.Unlock()
	st.Peers = d.PeerStatuses()
	return st
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
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid request JSON", http.StatusBadRequest)
			return
		}
		go func() {
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
