package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	source "fintex.au/mnet/internal/enrollmentsource"
)

func storeSourceFixture(t *testing.T, origin string) (registrarSourceConfig, contract.SourceReserveRequest, contract.SourceReserve) {
	t.Helper()
	raw, err := os.ReadFile("../enrollmentcontract/testdata/multies-provenance-source-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Exchange contract.SourceExchange `json:"exchange"`
		Reserve  contract.SourceReserve  `json:"reserve"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	m := fixture.Exchange.Mapping
	entry := registrarSourceEntry{ID: "fixture", SourceOrigin: origin, PrincipalAuthority: fixture.Reserve.Provenance.PrincipalAuthority, KeyID: "fixture-key", ServiceID: fixture.Reserve.ServiceID, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, Secret: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32)), TimeoutMS: 2000, MaxConnections: 1, Allowed: []contract.SourceMapping{m}}
	c := registrarSourceConfig{Version: 1, Revision: "r1", MaxConcurrent: 2, MaxPerSource: 1, MaxAttempts: 4, Sources: []registrarSourceEntry{entry}}
	return c, contract.SourceReserveRequest{Ticket: fixture.Exchange.Ticket, HandoffID: fixture.Exchange.ID, ReserveID: fixture.Reserve.ReserveID, Launch: fixture.Exchange.Launch, Mapping: m}, fixture.Reserve
}
func writeStoreSource(t *testing.T, s *Store, c registrarSourceConfig) []byte {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ownedPath+".registrar-sources-v1.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestRegistrarSourceConfigStrictRecursiveGrammar(t *testing.T) {
	c, _, _ := storeSourceFixture(t, "https://source.test")
	raw, _ := json.Marshal(c)
	if _, _, err := parseRegistrarSources(raw); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"null", "missing", "duplicate", "case", "unknown", "trailing", "version", "fraction", "secret", "origin", "alias", "limits", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			changed := append([]byte(nil), raw...)
			switch kind {
			case "null":
				changed = bytes.Replace(changed, []byte(`"sources":[`), []byte(`"sources":null,"ignored":[`), 1)
			case "missing":
				changed = bytes.Replace(changed, []byte(`"version":1,`), nil, 1)
			case "duplicate":
				changed = append([]byte(`{"version":1,`), changed[1:]...)
			case "case":
				changed = bytes.Replace(changed, []byte(`"revision"`), []byte(`"Revision"`), 1)
			case "unknown":
				changed = append([]byte(`{"other":1,`), changed[1:]...)
			case "trailing":
				changed = append(changed, []byte(`{}`)...)
			case "version":
				changed = bytes.Replace(changed, []byte(`"version":1`), []byte(`"version":2`), 1)
			case "fraction":
				changed = bytes.Replace(changed, []byte(`"version":1`), []byte(`"version":1.0`), 1)
			case "secret":
				changed = bytes.Replace(changed, []byte(c.Sources[0].Secret), []byte("invalid"), 1)
			case "origin":
				changed = bytes.Replace(changed, []byte("https://source.test"), []byte("http://source.test"), 1)
			case "alias":
				copyConfig := c
				copyConfig.Sources = append(append([]registrarSourceEntry(nil), c.Sources...), c.Sources[0])
				copyConfig.Sources[1].ID = "other"
				changed, _ = json.Marshal(copyConfig)
			case "limits":
				changed = bytes.Replace(changed, []byte(`"max_attempts":4`), []byte(`"max_attempts":0`), 1)
			case "timeout":
				changed = bytes.Replace(changed, []byte(`"timeout_ms":2000`), []byte(`"timeout_ms":60001`), 1)
			}
			if _, _, err := parseRegistrarSources(changed); err == nil {
				t.Fatal("invalid source schema accepted")
			}
		})
	}
}

// Recursive missing/null mutation uses the actual full root, not nested copies.
func TestRegistrarSourceConfigEveryFieldRequiredNonNull(t *testing.T) {
	c, _, _ := storeSourceFixture(t, "https://source.test")
	raw, _ := json.Marshal(c)
	var root any
	json.Unmarshal(raw, &root)
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, original := range v {
				v[key] = nil
				changed, _ := json.Marshal(root)
				if _, _, err := parseRegistrarSources(changed); err == nil {
					t.Fatal("null required field accepted", key)
				}
				delete(v, key)
				changed, _ = json.Marshal(root)
				if _, _, err := parseRegistrarSources(changed); err == nil {
					t.Fatal("missing required field accepted", key)
				}
				v[key] = original
				walk(original)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(root)
}
func TestRegistrarSourcesNoStartupReadAndIndependentStorePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path+".registrar-sources-v1.json", []byte("invalid"), 0600)
	s, err := Open(path)
	if err != nil {
		t.Fatal("Open read inactive source config", err)
	}
	defer s.Close()
	if s.registrarSources != nil {
		t.Fatal("source startup trigger")
	}
	second, err := Open(filepath.Join(dir, "second.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.mu.Lock()
	_, err = second.registrarSourcesLocked()
	second.mu.Unlock()
	if !errors.Is(err, errRegistrarSourceOff) {
		t.Fatal("distinct Store used first config", err)
	}
	if _, err := os.Stat(second.ownedPath + ".registrar-sources-v1.json"); !os.IsNotExist(err) {
		t.Fatal("missing config synthesized")
	}
	c, _, _ := storeSourceFixture(t, "https://source.test")
	writeStoreSource(t, s, c)
	s.mu.Lock()
	owner, err := s.registrarSourcesLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	again, err := s.registrarSourcesLocked()
	s.mu.Unlock()
	if err != nil || again != owner {
		t.Fatal("duplicate handle reset pool")
	}
}
func TestRegistrarSourceChangePermanentlyRetiresOnlySourceSubsystem(t *testing.T) {
	for _, kind := range []string{"bytes", "missing", "malformed", "symlink", "hardlink", "directory", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			s, err := Open(filepath.Join(t.TempDir(), "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			c, _, _ := storeSourceFixture(t, "https://source.test")
			raw := writeStoreSource(t, s, c)
			path := s.ownedPath + ".registrar-sources-v1.json"
			s.mu.Lock()
			owner, err := s.registrarSourcesLocked()
			s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "bytes":
				os.WriteFile(path, append(raw, ' '), 0600)
			case "missing":
				os.Remove(path)
			case "malformed":
				os.WriteFile(path, []byte(`{}`), 0600)
			case "symlink":
				os.Remove(path)
				target := filepath.Join(t.TempDir(), "other")
				os.WriteFile(target, raw, 0600)
				if err := os.Symlink(target, path); err != nil {
					t.Skip(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".link"); err != nil {
					t.Skip(err)
				}
			case "directory":
				os.Remove(path)
				os.Mkdir(path, 0700)
			case "oversize":
				os.WriteFile(path, bytes.Repeat([]byte(" "), registrarSourceMaxBytes+1), 0600)
			}
			s.mu.Lock()
			_, err = s.registrarSourcesLocked()
			s.mu.Unlock()
			if err == nil || !owner.retired {
				t.Fatal("changed config didn't retire pool")
			}
			if err := s.Health(); err != nil {
				t.Fatal("source policy retired unrelated Store authority", err)
			}
			os.Remove(path)
			os.WriteFile(path, raw, 0600)
			s.mu.Lock()
			_, err = s.registrarSourcesLocked()
			s.mu.Unlock()
			if err == nil || s.registrarSources != owner {
				t.Fatal("restored bytes reset spent history")
			}
		})
	}
}
func TestRegistrarSourceActualStoreTLSLifecycle(t *testing.T) {
	if os.Getenv("MNET_SOURCE_OWNER_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRegistrarSourceActualStoreTLSLifecycle$", "-test.v")
		cmd.Env = append(os.Environ(), "MNET_SOURCE_OWNER_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated TLS lifecycle test failed: %v\n%s", err, out)
		}
		return
	}
	var handler atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.Load().(http.HandlerFunc)(w, r) }))
	defer server.Close()
	cert := filepath.Join(t.TempDir(), "root.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	for _, kind := range []string{"success", "config-change", "close", "freeze", "lost"} {
		t.Run(kind, func(t *testing.T) {
			s, err := Open(filepath.Join(t.TempDir(), "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			c, request, response := storeSourceFixture(t, server.URL)
			rawConfig := writeStoreSource(t, s, c)
			var calls atomic.Int32
			handler.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				switch kind {
				case "config-change":
					os.WriteFile(s.ownedPath+".registrar-sources-v1.json", append(rawConfig, ' '), 0600)
				case "close":
					s.Close()
					return
				case "freeze":
					s.mu.Lock()
					s.freezeLocked(ErrStoreUncertain)
					s.mu.Unlock()
					return
				case "lost":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						conn.Close()
					}
					return
				}
				now := time.Now()
				response.CheckedAt = now.Add(-time.Millisecond).UTC().Format("2006-01-02T15:04:05.000Z")
				response.Deadline = now.Add(time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				json.NewEncoder(w).Encode(response)
			}))
			before, _ := os.ReadFile(s.ownedPath)
			got, err := s.reserveRegistrarSource(context.Background(), "fixture", request)
			if kind == "success" {
				if err != nil || got.ServiceID != response.ServiceID {
					t.Fatal("actual Store-owned source refused", err)
				}
			} else {
				if !errors.Is(err, source.ErrUnavailable) || got != (contract.SourceReserve{}) {
					t.Fatal("post-I/O lost/current source not unknown", err)
				}
			}
			if calls.Load() != 1 {
				t.Fatal("unexpected source sends")
			}
			if _, err := s.reserveRegistrarSource(context.Background(), "fixture", request); err == nil || calls.Load() != 1 {
				t.Fatal("actual owner repeated spent source attempt", err)
			}
			after, _ := os.ReadFile(s.ownedPath)
			if !bytes.Equal(before, after) || len(s.data.Nodes) != 0 || s.data.Registrar != nil {
				t.Fatal("source DATA mutated Store authority")
			}
		})
	}
}
