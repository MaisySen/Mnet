// Package control implements the Mnet control plane: node
// registration, approval, auth keys, network maps and the admin API.
package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/privatefs"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrBadAuthKey   = errors.New("invalid or expired auth key")
	ErrBadName      = errors.New("invalid name")
)

// ApprovalIdentity is provenance from a verified OIDC login, not network
// ownership or an authorization grant. It never goes into peer maps.
type ApprovalIdentity struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

type storeData struct {
	Admission          *registrarAdmissionLedger   `json:"admission_ledger,omitempty"`
	Registrar          *registrarState             `json:"registrar_state,omitempty"`
	Memberships        map[string]NetworkMember    `json:"memberships,omitempty"`
	ApprovalIdentities map[string]ApprovalIdentity `json:"approval_identities,omitempty"`
	Networks           map[string]*netmap.Network  `json:"networks"`
	Nodes              map[string]*netmap.Node     `json:"nodes"`
	AuthKeys           map[string]*netmap.AuthKey  `json:"auth_keys"`
	NodeTokens         map[string]string           `json:"node_tokens"`
	NextIP             map[string]int              `json:"next_ip"` // network ID -> host part offset
}

// Store is the whole control-plane state, persisted as one JSON file.
type Store struct {
	// Private instance seam for observing real proof allocation in failure tests.
	// No external selector; nil calls the existing proof.NewServer unchanged.
	registrarProofFactory func(contract.Binding, time.Time, time.Time, time.Duration) (*proof.Server, contract.Challenge, error)
	admissionPolicy       registrarAdmissionPolicy
	storageCodec          storeCodec // nil preserves exact legacy bytes; no public/config setter
	registrarIngress      *registrarIngressOwner
	registrarSources      *registrarSourceOwner
	registrarEnrollment   *registrarEnrollmentOwner
	mu                    sync.Mutex
	path                  string
	ownedPath             string
	lease                 *storeLease
	fs                    storeFilesystem
	closed                bool
	fault                 error
	data                  storeData
	version               int64
	relayIndex            map[string]string                 // network+public key -> node ID, invalidated on identity changes
	watchers              map[string]map[chan struct{}]bool // network ID -> waiters
}

// storeCodec is a private per-instance storage-format seam. Nil is legacy.
type storeCodec interface {
	decode([]byte) ([]byte, error)
	encode([]byte) ([]byte, error)
	maxBytes() int
}

// Open loads or creates the legacy-format store at path.
func Open(path string) (*Store, error) { return openStore(path, nil) }
func openStore(path string, codec storeCodec) (*Store, error) {
	lease, path, err := acquireStoreLease(path)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			lease.owner.Close()
		}
	}()
	s := &Store{
		storageCodec: codec,
		path:         path,
		ownedPath:    path, lease: lease, fs: defaultStoreFilesystem(),
		watchers: map[string]map[chan struct{}]bool{},
	}
	s.data = storeData{
		Networks:   map[string]*netmap.Network{},
		Nodes:      map[string]*netmap.Node{},
		AuthKeys:   map[string]*netmap.AuthKey{},
		NodeTokens: map[string]string{},
		NextIP:     map[string]int{},
	}
	if info, err := os.Lstat(path); err == nil {
		if err = singleLink(path, info); err != nil {
			return nil, err
		}
		if err = privatefs.ProtectFile(path); err != nil {
			return nil, fmt.Errorf("store: private state file required: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	var b []byte
	if codec == nil {
		b, err = os.ReadFile(path)
	} else {
		f, e := os.Open(path)
		if e != nil {
			err = e
		} else {
			b, err = io.ReadAll(io.LimitReader(f, int64(codec.maxBytes())+1))
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if len(b) > codec.maxBytes() {
				return nil, fmt.Errorf("store: bounded format too large")
			}
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		if codec != nil {
			return nil, fmt.Errorf("store: dedicated existing image required")
		}
		ok = true
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if codec != nil {
		b, err = codec.decode(b)
		if err != nil {
			return nil, err
		}
	}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("store: expected a state object; file was not changed")
	}
	if err := validateRegistrarEnvelope(b); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("store: corrupt data file: %w", err)
	}
	if err := s.validateAdmissionFormat(); err != nil {
		return nil, err
	}
	if err := s.validateLoadedShape(); err != nil {
		return nil, err
	}
	if err := s.validateRegistrarState(); err != nil {
		return nil, err
	}
	// Re-establish the loaded image under the retained owner before serving it.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := s.fs.syncDir(lease.parent); err != nil {
		return nil, err
	}
	if err := lease.check(); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
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

// Path returns the backing file path.
func (s *Store) Path() string { return s.path }

// Version returns the current global map version.
func (s *Store) Version() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// WaitForChange blocks until the network's map version exceeds since or the
// timeout passes. Returns the new version.
func (s *Store) WaitForChange(networkID string, since int64, timeout time.Duration) int64 {
	return s.WaitForChangeContext(context.Background(), networkID, since, timeout)
}

// WaitForChangeContext releases its watcher promptly when a client leaves.
func (s *Store) WaitForChangeContext(ctx context.Context, networkID string, since int64, timeout time.Duration) int64 {
	s.mu.Lock()
	if err := s.healthLocked(); err != nil {
		v := s.version
		s.mu.Unlock()
		return v
	}
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
	case <-ctx.Done():
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
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	for _, n := range s.data.Networks {
		if n.Name == name {
			return nil, fmt.Errorf("network %q already exists", name)
		}
	}
	n := &netmap.Network{ID: newID("net"), Name: name, CreatedAt: time.Now().UTC()}
	s.data.Networks[n.ID] = n
	if err := s.commitLocked(func() { delete(s.data.Networks, n.ID) }); err != nil {
		return nil, err
	}
	copy := cloneNetwork(n)
	return &copy, nil
}

// Networks lists every network.
func (s *Store) Networks() []*netmap.Network {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return nil
	}
	out := make([]*netmap.Network, 0, len(s.data.Networks))
	for _, n := range s.data.Networks {
		c := cloneNetwork(n)
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

// allocIPLocked assigns within the mesh prefix, wrapping safely when the
// saved cursor is stale or outside the usable range. Existing node addresses
// are never moved. Network and broadcast addresses are not assigned.
func (s *Store) allocIPLocked(networkID string) (string, error) {
	used := map[string]bool{}
	for _, n := range s.data.Nodes {
		if n.NetworkID == networkID {
			used[n.IPv4] = true
		}
	}
	offset, ip, err := nextMeshIP(s.data.NextIP[networkID], (1<<22)-2, used)
	if err != nil {
		return "", err
	}
	s.data.NextIP[networkID] = offset
	return ip, nil
}

// A separate bounded chooser permits small-pool exhaustion tests without
// manufacturing millions of devices. max is the last usable host offset.
func nextMeshIP(cursor, max int, used map[string]bool) (int, string, error) {
	if max < 1 || max > (1<<22)-2 {
		return 0, "", fmt.Errorf("invalid mesh address range")
	}
	if cursor < 0 || cursor >= max {
		cursor = 0
	}
	for checked := 0; checked < max; checked++ {
		cursor++
		if cursor > max {
			cursor = 1
		}
		value := uint32(0x64400000) + uint32(cursor)
		ip := net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value)).String()
		if !used[ip] {
			return cursor, ip, nil
		}
	}
	return 0, "", fmt.Errorf("mesh address space exhausted")
}

// The encoded key is also an identity index. Accept one canonical spelling
// so equivalent base64 forms cannot allocate an alias of an existing node.
func validPubKey(pk string) bool {
	b, err := base64.StdEncoding.Strict().DecodeString(pk)
	return err == nil && len(b) == 32 && base64.StdEncoding.EncodeToString(b) == pk
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
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	network := s.findNetworkLocked(req.Network)
	if network == nil {
		return nil, ErrNotFound
	}

	if s.registrarClaimLocked(network.ID, req.PubKey) != nil {
		return nil, ErrUnauthorized
	}

	// A WireGuard public key is public metadata, not proof of ownership.
	// Reusing a stored identity always needs its saved control credential,
	// even when a separate network-enrollment key is supplied.
	var node *netmap.Node
	for _, n := range s.data.Nodes {
		if n.NetworkID == network.ID && n.PubKey == req.PubKey {
			node = n
			break
		}
	}
	if node != nil && (req.NodeID != node.ID || req.NodeToken == "" || subtle.ConstantTimeCompare([]byte(s.data.NodeTokens[node.ID]), []byte(req.NodeToken)) != 1) {
		return nil, ErrUnauthorized
	}
	var previousNode netmap.Node
	existed := node != nil
	if existed {
		previousNode = cloneNode(node)
	}
	previousOffset, hadOffset := s.data.NextIP[network.ID]
	var usedKey *netmap.AuthKey
	var previousUses int
	approved := false
	if req.AuthKey != "" {
		k := s.findAuthKeyLocked(req.AuthKey)
		if k == nil || k.NetworkID != network.ID {
			return nil, ErrBadAuthKey
		}
		if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
			return nil, ErrBadAuthKey
		}
		if !k.Reusable && k.Uses >= 1 {
			return nil, ErrBadAuthKey
		}
		usedKey = k
		previousUses = k.Uses
		k.Uses++
		approved = true
	}
	if node == nil {
		ipv4, err := s.allocIPLocked(network.ID)
		if err != nil {
			if usedKey != nil {
				usedKey.Uses = previousUses
			}
			return nil, err
		}
		expiry := time.Now().UTC().Add(DefaultKeyExpiry)
		node = &netmap.Node{
			ID:        newID("node"),
			NetworkID: network.ID,
			Name:      s.uniqueNodeNameLocked(network.ID, name),
			PubKey:    req.PubKey,
			IPv4:      ipv4,
			CreatedAt: time.Now().UTC(),
			KeyExpiry: &expiry,
		}
		s.data.Nodes[node.ID] = node
		s.data.NodeTokens[node.ID] = newToken()
	}
	node.Approved = node.Approved || approved
	node.LastSeen = time.Now().UTC()
	if node.KeyExpired() {
		expiry := time.Now().UTC().Add(DefaultKeyExpiry)
		node.KeyExpiry = &expiry
		// Expiry revokes authorization, not identity. A saved node token
		// cannot silently bypass an admin expiration; require fresh approval
		// or a currently valid enrollment key.
		node.Approved = approved
	}
	node.ExitNode = req.ExitNode
	node.Routes = cleanRoutes(req.Routes)
	if err := s.commitLocked(func() {
		if existed {
			*node = previousNode
		} else {
			delete(s.data.Nodes, node.ID)
			delete(s.data.NodeTokens, node.ID)
		}
		if hadOffset {
			s.data.NextIP[network.ID] = previousOffset
		} else {
			delete(s.data.NextIP, network.ID)
		}
		if usedKey != nil {
			usedKey.Uses = previousUses
		}
	}); err != nil {
		return nil, err
	}
	s.relayIndex = nil
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
		suffix := fmt.Sprintf("-%d", i)
		stem := want
		// cleanName bounds the requested ASCII label to 32 bytes. Keep the
		// automatic collision suffix within that same public name contract.
		if len(stem)+len(suffix) > 32 {
			stem = stem[:32-len(suffix)]
		}
		stem = strings.TrimRight(stem, "-")
		cand := stem + suffix
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
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	if s.registrarNodeLocked(nodeID) != nil {
		return nil, ErrUnauthorized
	}
	n := s.data.Nodes[nodeID]
	if n == nil {
		return nil, ErrNotFound
	}
	if s.data.NodeTokens[nodeID] != token || token == "" {
		return nil, ErrUnauthorized
	}
	c := cloneNode(n)
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
	if err := s.healthLocked(); err != nil {
		return
	}
	if n := s.data.Nodes[nodeID]; n != nil {
		n.LastSeen = time.Now().UTC()
	}
}

// DefaultKeyExpiry is how long a node key stays valid after the first
// join. Admins re-enable an expired node by rejoining it, which issues a
// fresh expiry.
const DefaultKeyExpiry = 180 * 24 * time.Hour

// ExpireNode forces a node's key to expire now (admin action and tests).
func (s *Store) ExpireNode(nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return err
	}
	n := s.data.Nodes[nodeID]
	if n == nil {
		return ErrNotFound
	}
	previous := n.KeyExpiry
	claim := s.registrarNodeLocked(nodeID)
	oldPhase := ""
	if claim != nil {
		oldPhase = claim.Phase
		claim.Phase = "revoked"
	}
	past := time.Now().UTC().Add(-time.Minute)
	if claim != nil && past.UnixMilli() > claim.AccessExpiresAtMS {
		past = time.UnixMilli(claim.AccessExpiresAtMS).UTC()
	}
	n.KeyExpiry = &past
	if err := s.commitLocked(func() {
		n.KeyExpiry = previous
		if claim != nil {
			claim.Phase = oldPhase
		}
	}); err != nil {
		return err
	}
	s.bumpLocked(n.NetworkID)
	return nil
}

// cleanRoutes keeps only valid CIDRs.
func cleanRoutes(in []string) []string {
	var out []string
	for _, r := range in {
		if _, _, err := net.ParseCIDR(r); err == nil {
			out = append(out, r)
		}
	}
	return out
}

// SetEndpoints stores a node's candidate endpoints.
func (s *Store) SetEndpoints(nodeID string, eps []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setEndpointsLocked(nodeID, eps)
}

// Requires Store.mu; owner/current-access gate belongs to caller. Legacy
// SetEndpoints retains its exact historical behavior through this helper.
func (s *Store) setEndpointsLocked(nodeID string, eps []string) error {
	if err := s.healthLocked(); err != nil {
		return err
	}
	n := s.data.Nodes[nodeID]
	if n == nil {
		return ErrNotFound
	}

	clean := make([]string, 0, len(eps))
	for _, endpoint := range eps {
		if _, _, err := net.SplitHostPort(endpoint); err == nil {
			clean = append(clean, endpoint)
		}
	}
	sort.Strings(clean)
	clean = slices.Compact(clean)
	previous := append([]string(nil), n.Endpoints...)
	sort.Strings(previous)
	previous = slices.Compact(previous)
	oldSeen, oldEndpoints := n.LastSeen, n.Endpoints
	n.LastSeen = time.Now().UTC()
	if slices.Equal(previous, clean) {
		return nil
	}
	n.Endpoints = clean
	if err := s.commitLocked(func() { n.LastSeen = oldSeen; n.Endpoints = oldEndpoints }); err != nil {
		return err
	}
	s.bumpLocked(n.NetworkID)
	return nil
}

// Map builds the network map for a node.
// cloneNode makes a detached value, including all mutable nested data.
func cloneNode(n *netmap.Node) netmap.Node {
	c := *n
	c.Endpoints = append([]string(nil), n.Endpoints...)
	c.Routes = append([]string(nil), n.Routes...)
	if n.KeyExpiry != nil {
		expiry := *n.KeyExpiry
		c.KeyExpiry = &expiry
	}
	return c
}
func cloneNetwork(n *netmap.Network) netmap.Network {
	c := *n
	if n.ACL != nil {
		c.ACL = &netmap.ACLPolicy{Rules: append([]netmap.ACLRule(nil), n.ACL.Rules...)}
	}
	return c
}

func (s *Store) mapSnapshotLocked(nodeID string) (*netmap.Map, []netmap.Node, error) {
	if err := s.healthLocked(); err != nil {
		return nil, nil, err
	}
	self := s.data.Nodes[nodeID]
	if self == nil {
		return nil, nil, ErrNotFound
	}
	if s.registrarAccessDeniedLocked(nodeID) || !self.Approved || self.KeyExpired() {
		return nil, nil, ErrUnauthorized
	}
	network := s.data.Networks[self.NetworkID]
	if network == nil {
		return nil, nil, ErrNotFound
	}
	m := &netmap.Map{Version: s.version, Network: cloneNetwork(network), Self: cloneNode(self), Generated: time.Now().UTC()}
	// Snapshot only while holding the lock. ACL evaluation, sorting and
	// subsequent HTTP encoding work on owned values outside the store lock.
	peers := make([]netmap.Node, 0, len(s.data.Nodes))
	for _, n := range s.data.Nodes {
		if n.NetworkID == self.NetworkID && n.ID != self.ID && n.Approved && !n.KeyExpired() && !s.registrarAccessDeniedLocked(n.ID) {
			peers = append(peers, cloneNode(n))
		}
	}
	return m, peers, nil
}

func (s *Store) Map(nodeID string) (*netmap.Map, error) {
	s.mu.Lock()
	m, peers, err := s.mapSnapshotLocked(nodeID)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return finishMapSnapshot(m, peers), nil
}

// Finish ACL filtering on detached values, never on owner state.
func finishMapSnapshot(m *netmap.Map, peers []netmap.Node) *netmap.Map {
	visible := 0
	for i := range peers {
		if filterPeerInPlace(m.Network.ACL, &m.Self, &peers[i]) {
			peers[visible] = peers[i]
			visible++
		}
	}
	clear(peers[visible:])
	if visible > 0 {
		m.Peers = peers[:visible]
	}
	sort.Slice(m.Peers, func(i, j int) bool { return m.Peers[i].IPv4 < m.Peers[j].IPv4 })
	return m
}

// Network returns one network by ID or name, or nil.
func (s *Store) Network(idOrName string) *netmap.Network {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return nil
	}
	n := s.findNetworkLocked(idOrName)
	if n == nil {
		return nil
	}
	c := cloneNetwork(n)
	return &c
}

// SetACL validates and stores the network policy; nil clears it back to
// allow-all. Bumps the map version so long-polling nodes re-filter.
func (s *Store) SetACL(networkID string, pol *netmap.ACLPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return err
	}
	n := s.findNetworkLocked(networkID)
	if n == nil {
		return ErrNotFound
	}
	if err := s.validateACL(networkID, pol); err != nil {
		return err
	}
	if pol != nil && len(pol.Rules) == 0 {
		pol = nil
	}
	if pol != nil {
		pol = &netmap.ACLPolicy{Rules: append([]netmap.ACLRule(nil), pol.Rules...)}
	}
	previous := n.ACL
	n.ACL = pol
	if err := s.commitLocked(func() { n.ACL = previous }); err != nil {
		return err
	}
	s.bumpLocked(networkID)
	return nil
}

// ApproveNodeAs marks a node approved and records the SSO identity
// behind the approval.
func (s *Store) ApproveNodeAs(nodeID, user string) error {
	return s.approveNode(nodeID, &user, nil)
}

// ApproveNode marks a pending node approved.
func (s *Store) ApproveNode(nodeID string) error { return s.approveNode(nodeID, nil, nil) }

// ApproveNodeIdentity records an issuer-bound subject only after verified
// authentication. Display email remains a label, never the account key.
func (s *Store) ApproveNodeIdentity(nodeID, user, issuer, subject string) error {
	if issuer == "" || subject == "" {
		return fmt.Errorf("verified issuer and subject required")
	}
	return s.approveNode(nodeID, &user, &ApprovalIdentity{Issuer: issuer, Subject: subject})
}

// NodeApprovalIdentity returns a value copy for internal account groundwork.
// Absence in old stores stays unknown: never infer a principal from email.
func (s *Store) NodeApprovalIdentity(nodeID string) (ApprovalIdentity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return ApprovalIdentity{}, false
	}
	identity, ok := s.data.ApprovalIdentities[nodeID]
	return identity, ok
}

func (s *Store) approveNode(nodeID string, user *string, identity *ApprovalIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return err
	}
	if s.registrarNodeLocked(nodeID) != nil {
		return ErrUnauthorized
	}
	n := s.data.Nodes[nodeID]
	if n == nil {
		return ErrNotFound
	}
	oldApproved, oldUser := n.Approved, n.User
	oldIdentity, hadIdentity := s.data.ApprovalIdentities[nodeID]
	identityMapWasNil := s.data.ApprovalIdentities == nil
	n.Approved = true
	if user != nil {
		n.User = *user
	}
	if identity != nil {
		if s.data.ApprovalIdentities == nil {
			s.data.ApprovalIdentities = map[string]ApprovalIdentity{}
		}
		s.data.ApprovalIdentities[nodeID] = *identity
	}
	if err := s.commitLocked(func() {
		n.Approved = oldApproved
		n.User = oldUser
		if hadIdentity {
			s.data.ApprovalIdentities[nodeID] = oldIdentity
		} else {
			delete(s.data.ApprovalIdentities, nodeID)
		}
		if identityMapWasNil {
			s.data.ApprovalIdentities = nil
		}
	}); err != nil {
		return err
	}
	s.bumpLocked(n.NetworkID)
	return nil
}

// DeleteNode removes a node entirely.
func (s *Store) DeleteNode(nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return err
	}
	n := s.data.Nodes[nodeID]
	if n == nil {
		return ErrNotFound
	}
	claim := s.registrarNodeLocked(nodeID)
	oldPhase := ""
	if claim != nil {
		oldPhase = claim.Phase
		claim.Phase = "revoked"
	}
	token, hadToken := s.data.NodeTokens[nodeID]
	identity, hadIdentity := s.data.ApprovalIdentities[nodeID]
	delete(s.data.ApprovalIdentities, nodeID)
	delete(s.data.Nodes, nodeID)
	delete(s.data.NodeTokens, nodeID)
	if err := s.commitLocked(func() {
		s.data.Nodes[nodeID] = n
		if hadToken {
			s.data.NodeTokens[nodeID] = token
		}
		if claim != nil {
			claim.Phase = oldPhase
		}
		if hadIdentity {
			s.data.ApprovalIdentities[nodeID] = identity
		}
	}); err != nil {
		return err
	}
	s.relayIndex = nil
	s.bumpLocked(n.NetworkID)
	return nil
}

// CreateAuthKey mints a join key for a network.
func (s *Store) CreateAuthKey(networkID, note string, reusable bool, ttl time.Duration) (*netmap.AuthKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
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
	if err := s.commitLocked(func() { delete(s.data.AuthKeys, k.ID) }); err != nil {
		return nil, err
	}
	copy := cloneAuthKey(k)
	return &copy, nil
}

func cloneAuthKey(k *netmap.AuthKey) netmap.AuthKey {
	copy := *k
	if k.ExpiresAt != nil {
		expiry := *k.ExpiresAt
		copy.ExpiresAt = &expiry
	}
	return copy
}

// DeleteAuthKey removes a key.
func (s *Store) DeleteAuthKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return err
	}
	previous := s.data.AuthKeys[id]
	if previous == nil {
		return ErrNotFound
	}
	delete(s.data.AuthKeys, id)
	return s.commitLocked(func() { s.data.AuthKeys[id] = previous })
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
	if err := s.healthLocked(); err != nil {
		return &Snapshot{}
	}
	snap := &Snapshot{}
	for _, n := range s.data.Networks {
		c := cloneNetwork(n)
		snap.Networks = append(snap.Networks, &c)
	}
	for _, n := range s.data.Nodes {
		c := cloneNode(n)
		snap.Nodes = append(snap.Nodes, &c)
	}
	for _, k := range s.data.AuthKeys {
		c := cloneAuthKey(k)
		snap.AuthKeys = append(snap.AuthKeys, &c)
	}
	sort.Slice(snap.Networks, func(i, j int) bool { return snap.Networks[i].Name < snap.Networks[j].Name })
	sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].CreatedAt.Before(snap.Nodes[j].CreatedAt) })
	sort.Slice(snap.AuthKeys, func(i, j int) bool { return snap.AuthKeys[i].CreatedAt.Before(snap.AuthKeys[j].CreatedAt) })
	return snap
}

// RelayAllowed revalidates both identities and current ACL on each packet.
// A scoped key index avoids a fleet-wide scan on every datagram.
func (s *Store) RelayAllowed(sourceID, dstKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthLocked(); err != nil {
		return false
	}
	return s.relayAllowedLocked(sourceID, dstKey)
}

// Requires the SAME owning Store mutex, after lifecycle/credential validation.
func (s *Store) relayAllowedLocked(sourceID, dstKey string) bool {
	source := s.data.Nodes[sourceID]
	if source == nil || s.registrarAccessDeniedLocked(sourceID) || !source.Approved || source.KeyExpired() {
		return false
	}
	if s.relayIndex == nil {
		s.relayIndex = make(map[string]string, len(s.data.Nodes))
		for id, node := range s.data.Nodes {
			s.relayIndex[node.NetworkID+"\x00"+node.PubKey] = id
		}
	}
	dst := s.data.Nodes[s.relayIndex[source.NetworkID+"\x00"+dstKey]]
	if dst == nil || s.registrarAccessDeniedLocked(dst.ID) || !dst.Approved || dst.KeyExpired() || dst.NetworkID != source.NetworkID {
		return false
	}
	network := s.data.Networks[source.NetworkID]
	return network != nil && allowsPair(network.ACL, source, dst)
}
