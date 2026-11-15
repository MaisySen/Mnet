package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

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
}

func statePath(dir string) string { return filepath.Join(dir, "state.json") }

// LoadState reads the state file, or returns an empty state.
func LoadState(dir string) (*State, error) {
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), b, 0o600)
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
