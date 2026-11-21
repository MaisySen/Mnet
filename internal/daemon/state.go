package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// State is everything the daemon persists between runs.
type State struct {
	native         *nativeSavedCredential
	nativeDetached bool
	revision       [32]byte
	revisionPath   string
	revisionExists bool
	uncertain      bool
	PrivateKey     string `json:"private_key"` // WireGuard private key, base64
	PublicKey      string `json:"public_key"`
	Server         string `json:"server"`  // control plane base URL
	Network        string `json:"network"` // network name or ID
	NodeID         string `json:"node_id"`
	NodeToken      string `json:"node_token"`
	NodeName       string `json:"node_name"`
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
	owner, err := openStateOwner(dir)
	if err != nil {
		return nil, err
	}
	defer owner.lease.Close()
	s := publicState(owner.saved)
	return &s, nil
}

// Save reacquires the same stable daemon.lock and refuses stale detached images.
// A fresh State can create an absent file only. It never replaces an existing
// identity without a matching loaded byte revision. Uncertain instances refuse.
func (s *State) Save(dir string) error {
	if s.native != nil || s.nativeDetached {
		return ErrStateStale
	}
	if s.uncertain {
		return ErrStateUncertain
	}
	owner, err := openStateOwner(dir)
	if err != nil {
		return err
	}
	defer owner.lease.Close()
	if s.revisionPath != "" && s.revisionPath != owner.path {
		return ErrStateStale
	}
	if s.revisionExists != owner.saved.revisionExists || (s.revisionExists && s.revision != owner.saved.revision) {
		return ErrStateStale
	}
	if err := owner.save(s); err != nil {
		if owner.fault {
			s.uncertain = true
		}
		return err
	}
	return nil
}

// EnsureKey generates and saves a WireGuard identity if we have none.
func (s *State) EnsureKey(dir string) error {
	if s.uncertain {
		return ErrStateUncertain
	}
	if err := s.validateIdentity(); err != nil {
		return err
	}
	if s.PrivateKey != "" {
		owner, err := openStateOwner(dir)
		if err != nil {
			return err
		}
		defer owner.lease.Close()
		if !s.revisionExists || s.revisionPath != owner.path || s.revision != owner.saved.revision || !owner.saved.revisionExists || s.PrivateKey != owner.saved.PrivateKey || s.PublicKey != owner.saved.PublicKey {
			return ErrStateStale
		}
		return nil
	}
	return s.ensureKey(func() error { return s.Save(dir) })
}
func (s *State) ensureKey(save func() error) error {
	if err := s.validateIdentity(); err != nil {
		return err
	}
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
	if err := save(); err != nil {
		// A retry must not mistake an unpublished in-memory identity for a
		// durable saved pair and skip the write after disk failure.
		if !errors.Is(err, ErrStateUncertain) {
			s.PrivateKey = ""
			s.PublicKey = ""
		}
		return err
	}
	return nil
}

// Validate existing key material instead of treating a broken identity as a
// fresh install. Metadata-only legacy state is allowed when no credential or
// half-key pair exists; this never repairs or rotates an existing identity.
func (s *State) validateIdentity() error {
	if err := s.validateNativeCredential(); err != nil {
		return err
	}
	if s.NodeID != "" || s.NodeToken != "" {
		if strings.TrimSpace(s.NodeID) == "" || strings.TrimSpace(s.NodeToken) == "" || strings.TrimSpace(s.Server) == "" || strings.TrimSpace(s.Network) == "" {
			return fmt.Errorf("incomplete saved enrollment; identity file was not changed")
		}
	}
	if s.PrivateKey == "" && s.PublicKey == "" {
		if s.NodeID != "" || s.NodeToken != "" {
			return fmt.Errorf("saved device credential has no key pair; identity file was not changed")
		}
		return nil
	}
	private, err := base64.StdEncoding.DecodeString(s.PrivateKey)
	if err != nil || len(private) != 32 {
		return fmt.Errorf("invalid saved private key; identity file was not changed")
	}
	public, err := base64.StdEncoding.DecodeString(s.PublicKey)
	if err != nil || len(public) != 32 {
		return fmt.Errorf("invalid saved public key; identity file was not changed")
	}
	expected, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil || !bytes.Equal(expected, public) {
		return fmt.Errorf("saved key pair does not match; identity file was not changed")
	}
	return nil
}

// A saved node credential belongs to its enrolled control target. Passing new
// startup flags must not disclose it to a different server or silently move a
// device between networks. A separate state directory is a separate identity.
func (s *State) validateJoinTarget(server, network string) error {
	if s.native != nil || s.nativeDetached {
		return ErrStateStale
	}
	if s.NodeID == "" && s.NodeToken == "" {
		return nil
	}
	if (server != "" && strings.TrimRight(server, "/") != strings.TrimRight(s.Server, "/")) || (network != "" && network != s.Network) {
		return fmt.Errorf("device is already enrolled with a different server or network; use its saved target or a separate state directory for a new identity")
	}
	return nil
}

func (State) String() string   { return "[private native State]" }
func (State) GoString() string { return "[private native State]" }
