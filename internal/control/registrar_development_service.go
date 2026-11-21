//go:build mnet_enrollment_development || multies_native_fixture

package control

// A disposable DEVELOPMENT process owner, never a legacy server configuration.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"fintex.au/mnet/internal/privatefs"
	"fintex.au/mnet/internal/relay"
)

const developmentRootMarker = "MNET-DISPOSABLE-REGISTRAR-ONLY-ROOT-V1\n"
const developmentStoreName = "registrar-only.store"

// DisposableRegistrar deliberately does not expose legacy Store operations.
type DisposableRegistrar struct {
	store         *Store
	root, network string
}

func (d *DisposableRegistrar) Root() string      { return d.root }
func (d *DisposableRegistrar) NetworkID() string { return d.network }
func (d *DisposableRegistrar) Close() error      { return d.store.Close() }

// NewDisposableRegistrar always creates a fresh random private directory. It
// never adopts an existing directory or imports a legacy network/state file.
func NewDisposableRegistrar(parent string) (*DisposableRegistrar, error) {
	root, e := os.MkdirTemp(parent, "mnet-disposable-registrar-")
	if e != nil {
		return nil, e
	}
	if e = privatefs.EnsureDir(root); e != nil {
		return nil, e
	}
	s, e := Open(filepath.Join(root, developmentStoreName))
	if e != nil {
		return nil, e
	}
	// No legacy bytes were published: select envelope BEFORE the first mutation.
	s.storageCodec = developmentRegistrarCodec{}
	fail := func(err error) (*DisposableRegistrar, error) { s.Close(); return nil, err }
	network, e := s.CreateNetwork("disposable-registrar-only")
	if e != nil {
		return fail(e)
	}
	if e = privatefs.WriteFile(filepath.Join(root, "root-kind"), []byte(developmentRootMarker)); e != nil {
		return fail(e)
	}
	if e = syncStoreDir(root); e != nil {
		return fail(e)
	}
	return &DisposableRegistrar{s, root, network.ID}, nil
}

// Root markers are local compatibility metadata, not authority or a same-UID
// defense. The actual non-JSON image independently rejects the legacy loader.
func OpenDisposableRegistrar(root string) (*DisposableRegistrar, error) {
	info, e := os.Lstat(root)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, fmt.Errorf("private marked registrar root required")
	}
	marker := filepath.Join(root, "root-kind")
	info, e = os.Lstat(marker)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != int64(len(developmentRootMarker)) || singleLink(marker, info) != nil {
		return nil, fmt.Errorf("private registrar root marker required")
	}
	raw, e := os.ReadFile(marker)
	if e != nil || !bytes.Equal(raw, []byte(developmentRootMarker)) {
		return nil, fmt.Errorf("registrar root kind mismatch")
	}
	s, e := openStore(filepath.Join(root, developmentStoreName), developmentRegistrarCodec{})
	if e != nil {
		return nil, e
	}
	network, e := developmentRegistrarOnly(s)
	if e != nil {
		s.Close()
		return nil, e
	}
	return &DisposableRegistrar{s, root, network}, nil
}
func developmentRegistrarOnly(s *Store) (string, error) {
	if len(s.data.Networks) != 1 || len(s.data.AuthKeys) != 0 || len(s.data.NodeTokens) != 0 || len(s.data.ApprovalIdentities) != 0 || len(s.data.Memberships) != 0 {
		return "", fmt.Errorf("registrar-only identities required; no conversion")
	}
	var network string
	for id := range s.data.Networks {
		network = id
	}
	for _, n := range s.data.Nodes {
		r := s.registrarNodeLocked(n.ID)
		if r == nil || r.NetworkID != network {
			return "", fmt.Errorf("legacy node cannot be adopted")
		}
	}
	if s.data.Registrar != nil {
		for _, r := range s.data.Registrar.Records {
			if r.NetworkID != network {
				return "", fmt.Errorf("foreign historical network")
			}
		}
	}
	// NextIP legitimately advances during registrar commits and is not authority.
	for id := range s.data.NextIP {
		if id != network {
			return "", fmt.Errorf("foreign allocation namespace")
		}
	}
	return network, nil
}

// Configure is initial-only DATA installation into this disposable private root.
// It does not prove operator authority, create grants, rotate or migrate config.
func (d *DisposableRegistrar) Configure(sources, workflow []byte, origin string) error {
	if _, e := d.validateDevelopmentConfiguration(sources, workflow, origin); e != nil {
		return e
	}
	for _, suffix := range []string{".registrar-sources-v1.json", ".registrar-workflow-v1.json"} {
		if _, e := os.Lstat(d.store.path + suffix); !os.IsNotExist(e) {
			return fmt.Errorf("initial-only configuration required")
		}
	}
	for _, entry := range []struct {
		suffix string
		raw    []byte
	}{{".registrar-sources-v1.json", sources}, {".registrar-workflow-v1.json", workflow}} {
		if e := privatefs.WriteFile(d.store.path+entry.suffix, entry.raw); e != nil {
			return e
		}
	}
	return syncStoreDir(d.root)
}

func (d *DisposableRegistrar) validateDevelopmentConfiguration(sources, workflow []byte, origin string) (registrarWorkflowConfig, error) {
	sourceEntries, limits, e := parseRegistrarSources(sources)
	if e != nil {
		return registrarWorkflowConfig{}, e
	}
	policy, _, e := parseRegistrarWorkflow(workflow)
	if e != nil {
		return registrarWorkflowConfig{}, e
	}
	if !developmentLoopbackOrigin(origin) || !developmentLoopbackOrigin(policy.Policy.SourceOrigin) {
		return registrarWorkflowConfig{}, fmt.Errorf("disposable loopback-only origins required")
	}
	if len(sourceEntries) != 1 || sourceEntries[0].ID != policy.SourceID || policy.Policy.RegistrarOrigin != origin || sourceEntries[0].Scope.RegistrarOrigin != origin || sourceEntries[0].Scope.ServiceID != policy.Policy.ServiceID {
		return registrarWorkflowConfig{}, fmt.Errorf("exact disposable registrar/source binding required")
	}

	if policy.ClockUncertaintyMS > 1 || policy.ChallengeMS > 10000 || policy.MaxLeaseMS > 15000 || policy.MaxObservationAgeMS > 20000 || policy.MaxLivePerPrincipal > 4 || policy.MaxProofAttempts > 4 || policy.MaxLive > 4 || policy.MaxAttempts > 8 || policy.MaxRecords > 8 || policy.MaxStateBytes > 1048576 || limits.MaxConcurrent > 2 || limits.MaxPerSource > 1 || limits.MaxAttempts > 8 || sourceEntries[0].Transport.Timeout > 3*time.Second || sourceEntries[0].Transport.MaxConnections > 1 {
		return registrarWorkflowConfig{}, fmt.Errorf("original disposable finite profile exceeded")
	}
	scope := sourceEntries[0].Scope
	p := policy.Policy
	if scope.SourceOrigin != p.SourceOrigin || scope.PrincipalAuthority != p.PrincipalAuthority || scope.Issuer != p.IssuerLabel || scope.Audience != p.Audience {
		return registrarWorkflowConfig{}, fmt.Errorf("source/policy scope mismatch")
	}
	_, checked, e := parseRegistrarWorkflow(workflow)
	if e != nil {
		return registrarWorkflowConfig{}, e
	}
	expected := checked.SourceMappings()
	if len(expected) != len(sourceEntries[0].Allowed) {
		return registrarWorkflowConfig{}, fmt.Errorf("exact source mappings required")
	}
	for _, m := range sourceEntries[0].Allowed {
		found := false
		for _, v := range expected {
			if m == v {
				found = true
			}
		}
		if !found {
			return registrarWorkflowConfig{}, fmt.Errorf("source mapping mismatch")
		}
	}
	for _, binding := range policy.Policy.Bindings {
		if binding.NetworkID != d.network {
			return registrarWorkflowConfig{}, fmt.Errorf("foreign network policy")
		}
	}
	for _, mapping := range sourceEntries[0].Allowed {
		if mapping.NetworkID != d.network {
			return registrarWorkflowConfig{}, fmt.Errorf("foreign source network")
		}
	}

	return policy, nil
}

// Handler retains exactly one existing finite gate. RAM counters reset on a
// process restart: this is not durable admission, quotas, renewal or hosting.
func (d *DisposableRegistrar) Handler() (http.Handler, error) {
	if _, e := developmentRegistrarOnly(d.store); e != nil {
		return nil, e
	}
	raw, e := readRegistrarSources(d.store.path + ".registrar-workflow-v1.json")
	if e != nil {
		return nil, e
	}
	policy, _, e := parseRegistrarWorkflow(raw)
	if e != nil {
		return nil, e
	}
	sourceRaw, e := readRegistrarSources(d.store.path + ".registrar-sources-v1.json")
	if e != nil {
		return nil, e
	}
	policy, e = d.validateDevelopmentConfiguration(sourceRaw, raw, policy.Policy.RegistrarOrigin)
	if e != nil {
		return nil, e
	}
	gate, e := d.store.newRegistrarDevelopmentIngress(registrarIngressLimits{MaxRequests: 64, MaxConcurrent: 4, MaxBodyBytes: 16384, MaxStateBytes: 4 << 20, MaxEntries: 8, Timeout: 5 * time.Second}, d.store.registrarDevelopmentProtocol(policy.Policy.ServiceID))
	if e != nil {
		return nil, e
	}
	relayHandler, e := d.store.newRegistrarDevelopmentRelayIngress(gate, relay.NewHub(), 256)
	if e != nil {
		return nil, e
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == registrarRelayPath {
			relayHandler.ServeHTTP(w, r)
			return
		}
		gate.ServeHTTP(w, r)
	}), nil
}

// Public process input intentionally uses opaque RawMessage, followed by the
// existing exact parsers; never format/log service secrets from configuration.
type DisposableRegistrarConfiguration struct {
	Sources  json.RawMessage `json:"sources"`
	Workflow json.RawMessage `json:"workflow"`
}

func developmentLoopbackOrigin(origin string) bool {
	u, e := url.Parse(origin)
	return e == nil && u.Scheme == "https" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" && u.Port() != "" && net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback()
}

// DecodeDisposableRegistrarConfiguration preserves both subdocuments exactly;
// reject duplicated/case-alias/unknown/missing top fields before owning parsers.
func DecodeDisposableRegistrarConfiguration(raw []byte) (DisposableRegistrarConfiguration, error) {
	var out DisposableRegistrarConfiguration
	bad := fmt.Errorf("bounded exact disposable configuration required")
	if len(raw) == 0 || len(raw) > 131072 {
		return out, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return out, bad
	}
	seen := map[string]bool{}
	for d.More() {
		tok, e = d.Token()
		name, ok := tok.(string)
		if e != nil || !ok || seen[name] || (name != "sources" && name != "workflow") {
			return out, bad
		}
		seen[name] = true
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return out, bad
		}
		if name == "sources" {
			out.Sources = v
		} else {
			out.Workflow = v
		}
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim('}') || len(seen) != 2 {
		return DisposableRegistrarConfiguration{}, bad
	}
	if _, e = d.Token(); e != io.EOF {
		return DisposableRegistrarConfiguration{}, bad
	}
	return out, nil
}
