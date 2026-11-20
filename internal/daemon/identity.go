package daemon

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// PrepareIdentity makes a durable local key before account sign in. It never
// chooses a control target, enrolls, starts a tunnel or changes routing. The
// lifecycle lock keeps key publication separate from Up/Down/Close ownership.
func (d *Daemon) PrepareIdentity() (string, error) {
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return "", errors.New("daemon is closed")
	}
	if d.state.PrivateKey == "" && d.status != "down" {
		return "", errors.New("identity preparation requires a stopped daemon")
	}
	if err := d.state.EnsureKey(d.cfg.StateDir); err != nil {
		return "", errors.New("local identity is unavailable; existing identity was not replaced")
	}
	return d.state.PublicKey, nil
}
func (d *Daemon) servePrepareIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "no identity parameters accepted", http.StatusBadRequest)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(raw) != 0 {
		http.Error(w, "no identity parameters accepted", http.StatusBadRequest)
		return
	}
	key, err := d.PrepareIdentity()
	if err != nil {
		http.Error(w, "local identity unavailable", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"public_key": key})
}
