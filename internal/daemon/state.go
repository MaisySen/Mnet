package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"fintex.au/mnet/internal/privatefs"
	"golang.org/x/crypto/curve25519"
)

// State is everything the daemon persists between runs.
type State struct {
	PrivateKey string `json:"private_key"` // WireGuard private key, base64
	PublicKey  string `json:"public_key"`
	Server     string `json:"server"`  // control plane base URL
	Network    string `json:"network"` // network name or ID
	NodeID     string `json:"node_id"`
	NodeToken  string `json:"node_token"`
	NodeName   string `json:"node_name"`
	// AuthKey is the pre-auth key used at join, kept so the node can
	// re-join by itself when the control plane forgets it (state wipe
	// on the server). Same sensitivity as the private key beside it.
	AuthKey string `json:"auth_key,omitempty"`

	ExitNode        bool     `json:"exit_node,omitempty"`        // offer to carry others' internet traffic
	AdvertiseRoutes []string `json:"advertise_routes,omitempty"` // subnet CIDRs this node serves
	UseExitNode     string   `json:"use_exit_node,omitempty"`    // peer name or mesh IP to send internet traffic via
}

func statePath(dir string) string { return filepath.Join(dir, "state.json") }

// LoadState reads the state file, or returns an empty state.
func LoadState(dir string) (*State, error) {
	if _, err := os.Lstat(statePath(dir)); err == nil {
		if err = privatefs.EnsureDir(dir); err != nil {
			return nil, err
		}
		if err = privatefs.ProtectFile(statePath(dir)); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	b, err := os.ReadFile(statePath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes the state file, owner readable only.
func (s *State) Save(dir string) error {
	if err := privatefs.EnsureDir(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return privatefs.WriteFile(statePath(dir), b)
}

// EnsureKey generates and saves a WireGuard identity if we have none.
func (s *State) EnsureKey(dir string) error {
	if s.PrivateKey != "" {
		return nil
	}
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return err
	}
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return err
	}
	s.PrivateKey = base64.StdEncoding.EncodeToString(priv[:])
	s.PublicKey = base64.StdEncoding.EncodeToString(pub)
	return s.Save(dir)
}
