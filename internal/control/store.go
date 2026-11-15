// Package control implements the Mnet control plane: node
// registration, approval, auth keys, network maps and the admin API.
package control

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"fintex.au/mnet/internal/netmap"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrBadAuthKey   = errors.New("invalid or expired auth key")
	ErrBadName      = errors.New("invalid name")
)

type storeData struct {
	Networks   map[string]*netmap.Network `json:"networks"`
	Nodes      map[string]*netmap.Node    `json:"nodes"`
	AuthKeys   map[string]*netmap.AuthKey `json:"auth_keys"`
	NodeTokens map[string]string          `json:"node_tokens"`
	NextIP     map[string]int             `json:"next_ip"` // network ID -> host part offset
}

// Store is the whole control-plane state, persisted as one JSON file.
type Store struct {
	mu       sync.Mutex
	path     string
	data     storeData
	version  int64
	watchers map[string]map[chan struct{}]bool // network ID -> waiters
}

// Open loads or creates the store at path.
func Open(path string) (*Store, error) {
	s := &Store{
		path:     path,
		watchers: map[string]map[chan struct{}]bool{},
	}
	s.data = storeData{
		Networks:   map[string]*netmap.Network{},
		Nodes:      map[string]*netmap.Node{},
		AuthKeys:   map[string]*netmap.AuthKey{},
		NodeTokens: map[string]string{},
		NextIP:     map[string]int{},
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("store: corrupt data file: %w", err)
		}
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID(prefix string) string {
	b := make([]byte, 12)
	rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func newToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "mnet_" + base64.RawURLEncoding.EncodeToString(b)
}

// bump records a network change and wakes map waiters. Caller holds the lock.
func (s *Store) bumpLocked(networkID string) {
	s.version++
	for ch := range s.watchers[networkID] {
		close(ch)
		delete(s.watchers[networkID], ch)
	}
}

// Version returns the current global map version.
func (s *Store) Version() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// WaitForChange blocks until the network's map version exceeds since or the
// timeout passes. Returns the new version.
func (s *Store) WaitForChange(networkID string, since int64, timeout time.Duration) int64 {
	s.mu.Lock()
	if s.version > since {
		v := s.version
		s.mu.Unlock()
		return v
	}
	ch := make(chan struct{})
	if s.watchers[networkID] == nil {
		s.watchers[networkID] = map[chan struct{}]bool{}
	}
	s.watchers[networkID][ch] = true
	s.mu.Unlock()

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.watchers[networkID], ch)
	return s.version
}

// CreateNetwork adds a network. Name must be DNS safe.
func (s *Store) CreateNetwork(name string) (*netmap.Network, error) {
	name, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.data.Networks {
		if n.Name == name {
			return nil, fmt.Errorf("network %q already exists", name)
		}
	}
	n := &netmap.Network{ID: newID("net"), Name: name, CreatedAt: time.Now().UTC()}
	s.data.Networks[n.ID] = n
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return n, nil
}

// Networks lists every network.
func (s *Store) Networks() []*netmap.Network {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*netmap.Network, 0, len(s.data.Networks))
	for _, n := range s.data.Networks {
		c := *n
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var nameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

func cleanName(name string) (string, error) {
	n := []byte(name)
	for i, c := range n {
		if c >= 'A' && c <= 'Z' {
			n[i] = c + 32
		}
	}
	name = string(n)
	if !nameRE.MatchString(name) {
		return "", ErrBadName
	}
	return name, nil
}

func (s *Store) findNetworkLocked(idOrName string) *netmap.Network {
	if n := s.data.Networks[idOrName]; n != nil {
		return n
	}
	name, err := cleanName(idOrName)
	if err != nil {
		return nil
	}
	for _, n := range s.data.Networks {
		if n.Name == name {
			return n
		}
	}
	return nil
}

// allocIPLocked assigns the next free 100.64.0.0/10 address in the network.
func (s *Store) allocIPLocked(networkID string) string {
	base := net.ParseIP(netmap.VPNBase).To4()
	off := s.data.NextIP[networkID]
	used := map[string]bool{}
	for _, n := range s.data.Nodes {
		if n.NetworkID == networkID {
			used[n.IPv4] = true
		}
	}
	for {
		off++
		v := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
		v += uint32(off) - 1
		ip := net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String()
		if !used[ip] {
			s.data.NextIP[networkID] = off
			return ip
		}
	}
}

func validPubKey(pk string) bool {
	b, err := base64.StdEncoding.DecodeString(pk)
	return err == nil && len(b) == 32
}

// Join registers (or refreshes) a node. With a valid auth key the node is
// approved at once; otherwise it waits for admin approval.
func (s *Store) Join(req netmap.JoinRequest) (*netmap.JoinResponse, error) {
	if !validPubKey(req.PubKey) {
		return nil, fmt.Errorf("invalid wireguard public key")
	}
	name, err := cleanName(req.Name)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	network := s.findNetworkLocked(req.Network)
	if network == nil {
		return nil, ErrNotFound
	}

	approved := false
	if req.AuthKey != "" {
		k := s.findAuthKeyLocked(req.AuthKey)
		if k == nil || k.NetworkID != network.ID {
			return nil, ErrBadAuthKey
		}
		if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
			return nil, ErrBadAuthKey
		}
		k.Uses++
		if !k.Reusable && k.Uses > 1 {
			return nil, ErrBadAuthKey
		}
		approved = true
	}

	// Same device rejoining keeps its identity and address.
	var node *netmap.Node
	for _, n := range s.data.Nodes {
		if n.NetworkID == network.ID && n.PubKey == req.PubKey {
			node = n
			break
		}
	}
	if node == nil {
		node = &netmap.Node{
			ID:        newID("node"),
			NetworkID: network.ID,
			Name:      s.uniqueNodeNameLocked(network.ID, name),
			PubKey:    req.PubKey,
			IPv4:      s.allocIPLocked(network.ID),
			CreatedAt: time.Now().UTC(),
		}
		s.data.Nodes[node.ID] = node
		s.data.NodeTokens[node.ID] = newToken()
	}
	node.Approved = node.Approved || approved
	node.LastSeen = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	s.bumpLocked(network.ID)
	return &netmap.JoinResponse{
		NodeID:    node.ID,
		NodeToken: s.data.NodeTokens[node.ID],
		Approved:  node.Approved,
	}, nil
}

func (s *Store) uniqueNodeNameLocked(networkID, want string) string {
	taken := map[string]bool{}
	for _, n := range s.data.Nodes {
		if n.NetworkID == networkID {
			taken[n.Name] = true
		}
	}
	if !taken[want] {
		return want
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", want, i)
		if !taken[cand] {
			return cand
		}
	}
}

func (s *Store) findAuthKeyLocked(key string) *netmap.AuthKey {
	for _, k := range s.data.AuthKeys {
		if k.Key == key {
			return k
		}
	}
	return nil
}

// AuthorizeNode checks a node ID plus token pair.
func (s *Store) AuthorizeNode(nodeID, token string) (*netmap.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.data.Nodes[nodeID]
	if n == nil {
		return nil, ErrNotFound
	}
	if s.data.NodeTokens[nodeID] != token || token == "" {
		return nil, ErrUnauthorized
	}
	c := *n
	return &c, nil
}

// JoinStatus reports whether a node is approved yet.
func (s *Store) JoinStatus(nodeID, token string) (*netmap.JoinStatus, error) {
	n, err := s.AuthorizeNode(nodeID, token)
	if err != nil {
		return nil, err
	}
	return &netmap.JoinStatus{Approved: n.Approved, IPv4: n.IPv4}, nil
}

// Touch refreshes LastSeen.
func (s *Store) Touch(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := s.data.Nodes[nodeID]; n != nil {
		n.LastSeen = time.Now().UTC()
	}
}

// SetEndpoints stores a node's candidate endpoints.
func (s *Store) SetEndpoints(nodeID string, eps []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.data.Nodes[nodeID]
	if n == nil {
		return ErrNotFound
	}
	clean := make([]string, 0, len(eps))
	for _, e := range eps {
		if _, _, err := net.SplitHostPort(e); err == nil {
			clean = append(clean, e)
		}
	}
	n.Endpoints = clean
	n.LastSeen = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.bumpLocked(n.NetworkID)
	return nil
}

// Map builds the network map for a node.
func (s *Store) Map(nodeID string) (*netmap.Map, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	self := s.data.Nodes[nodeID]
	if self == nil {
		return nil, ErrNotFound
	}
	network := s.data.Networks[self.NetworkID]
	if network == nil {
		return nil, ErrNotFound
	}
	m := &netmap.Map{
		Version:   s.version,
		Network:   *network,
		Self:      *self,
		Generated: time.Now().UTC(),
	}
	for _, n := range s.data.Nodes {
		if n.NetworkID == self.NetworkID && n.ID != self.ID && n.Approved {
			c := *n
			m.Peers = append(m.Peers, c)
		}
	}
	sort.Slice(m.Peers, func(i, j int) bool { return m.Peers[i].IPv4 < m.Peers[j].IPv4 })
	return m, nil
}

// ApproveNode marks a pending node approved.
func (s *Store) ApproveNode(nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.data.Nodes[nodeID]
	if n == nil {
		return ErrNotFound
	}
	n.Approved = true
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.bumpLocked(n.NetworkID)
	return nil
}

// DeleteNode removes a node entirely.
func (s *Store) DeleteNode(nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.data.Nodes[nodeID]
	if n == nil {
		return ErrNotFound
	}
	delete(s.data.Nodes, nodeID)
	delete(s.data.NodeTokens, nodeID)
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.bumpLocked(n.NetworkID)
	return nil
}

// CreateAuthKey mints a join key for a network.
func (s *Store) CreateAuthKey(networkID, note string, reusable bool, ttl time.Duration) (*netmap.AuthKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Networks[networkID] == nil {
		return nil, ErrNotFound
	}
	k := &netmap.AuthKey{
		ID:        newID("key"),
		NetworkID: networkID,
		Key:       newToken(),
		Note:      note,
		Reusable:  reusable,
		CreatedAt: time.Now().UTC(),
	}
	if ttl > 0 {
		t := time.Now().Add(ttl).UTC()
		k.ExpiresAt = &t
	}
	s.data.AuthKeys[k.ID] = k
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return k, nil
}

// DeleteAuthKey removes a key.
func (s *Store) DeleteAuthKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.AuthKeys[id] == nil {
		return ErrNotFound
	}
	delete(s.data.AuthKeys, id)
	return s.saveLocked()
}

// Snapshot is the admin view of the world.
type Snapshot struct {
	Networks []*netmap.Network `json:"networks"`
	Nodes    []*netmap.Node    `json:"nodes"`
	AuthKeys []*netmap.AuthKey `json:"auth_keys"`
}

// Snapshot returns a copy of all state for the admin API.
func (s *Store) Snapshot() *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := &Snapshot{}
	for _, n := range s.data.Networks {
		c := *n
		snap.Networks = append(snap.Networks, &c)
	}
	for _, n := range s.data.Nodes {
		c := *n
		snap.Nodes = append(snap.Nodes, &c)
	}
	for _, k := range s.data.AuthKeys {
		c := *k
		snap.AuthKeys = append(snap.AuthKeys, &c)
	}
	sort.Slice(snap.Networks, func(i, j int) bool { return snap.Networks[i].Name < snap.Networks[j].Name })
	sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].CreatedAt.Before(snap.Nodes[j].CreatedAt) })
	sort.Slice(snap.AuthKeys, func(i, j int) bool { return snap.AuthKeys[i].CreatedAt.Before(snap.AuthKeys[j].CreatedAt) })
	return snap
}
