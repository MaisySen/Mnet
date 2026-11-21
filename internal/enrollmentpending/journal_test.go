package enrollmentpending

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func prepared(t *testing.T) (*Journal, contract.SourceLaunch, time.Time, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	now := time.Unix(1800000000, 0)
	key := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	mapping := contract.SourceMapping{WorkspaceID: "org_test", Issuer: "https://issuer.test/path", RegistrarOrigin: "https://registrar.test", Audience: "fixture", MappingRevision: "r1", NetworkID: "net_test"}
	launch, err := j.Begin(key, "linux", "http://127.0.0.1:34567/mnet/enrollment/callback", fixturePolicy(t, "https://source.test", mapping), now, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return j, launch, now, dir
}
func code() string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)) }
func exchange(t *testing.T, j *Journal, now time.Time) []byte {
	t.Helper()
	r := j.data
	e := contract.SourceExchange{ResponseVersion: 2, Provenance: contract.SourceProvenance{PrincipalAuthority: r.Authority, PrincipalNamespace: contract.PrincipalNamespace, PrincipalID: "fixture-local-user", SessionBinding: code()}, Launch: r.Launch, Ticket: code(), ID: code(), ExpiresAt: now.Add(time.Minute).UTC().Format("2006-01-02T15:04:05.000Z"), Mapping: r.Allowed[0], Purpose: "mnet-enrollment"}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestSavedOwnMaterialAndExplicitRemoteAmbiguitySurviveReopen(t *testing.T) {
	j, launch, now, dir := prepared(t)
	credential, verifier := j.data.Credential, j.data.Verifier
	raw, err := os.ReadFile(j.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(credential)) || launch.CredentialDigest != hash(credential) || launch.Challenge != hash(verifier) {
		t.Fatal("own material not durably pinned")
	}

	if err := j.RecordCallback(launch.AttemptID, launch.State, code(), now); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if j.data.Credential != credential || j.data.Verifier != verifier || j.data.Launch != launch {
		t.Fatal("restart changed saved launch or native secrets")
	}
	if phase, err := j.Status(); err != nil || phase != "callback-recorded" {
		t.Fatal("restart lost recorded callback", err)
	}
	if _, err := j.Begin(launch.DevicePublicKey, launch.Platform, launch.RedirectURI, savedPolicy(t, j), now, now.Add(time.Minute)); !errors.Is(err, ErrPending) {
		t.Fatal("silently replaced intent", err)
	}
	if err := j.RecordCallback(launch.AttemptID, launch.State, code(), now); err == nil {
		t.Fatal("second callback accepted")
	}
	if err := j.MarkExchangeStarted(now); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if phase, err := j.Status(); err != nil || phase != "exchange-may-have-started" {
		t.Fatal("remote response-loss uncertainty disappeared", err)
	}
	if err := j.MarkExchangeStarted(now); err == nil {
		t.Fatal("restart marked remote retry safe")
	}
	if err := j.RecordExchange(exchange(t, j, now), savedConfig(t, j), now); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if phase, err := j.Status(); err != nil || phase != "exchanged-data" {
		t.Fatal("response not durable DATA", err)
	}
}
func TestCallbackDeadlinesOriginalBindingAndCurrentConfig(t *testing.T) {
	j, l, now, _ := prepared(t)
	for _, p := range []struct {
		attempt, state, code string
		now                  time.Time
	}{{code(), l.State, code(), now}, {l.AttemptID, code(), code(), now}, {l.AttemptID, l.State, "bad", now}, {l.AttemptID, l.State, code(), now.Add(2 * time.Minute)}, {l.AttemptID, l.State, code(), now.Add(-time.Second)}} {
		if err := j.RecordCallback(p.attempt, p.state, p.code, p.now); err == nil {
			t.Fatal("changed/expired callback accepted")
		}
	}
	if err := j.RecordCallback(l.AttemptID, l.State, code(), now); err != nil {
		t.Fatal(err)
	}
	if err := j.RecordExchange(exchange(t, j, now), savedConfig(t, j), now); err == nil {
		t.Fatal("exchange accepted without publishing remote ambiguity")
	}
	if err := j.MarkExchangeStarted(now); err != nil {
		t.Fatal(err)
	}
	if err := j.RecordExchange(exchange(t, j, now), SourceConfig{}, now); err == nil {
		t.Fatal("removed config accepted")
	}
	if phase, err := j.Status(); err != nil || phase != "exchange-may-have-started" {
		t.Fatal("invalid response discarded uncertainty", err)
	}
}

var injected = errors.New("injected operation failure")

type wrappedTemp struct {
	tempFile
	stage string
}

func (f *wrappedTemp) Write(b []byte) (int, error) {
	if f.stage == "write" {
		return 0, injected
	}
	if f.stage == "short" {
		return f.tempFile.Write(b[:len(b)/2])
	}
	return f.tempFile.Write(b)
}
func (f *wrappedTemp) Sync() error {
	if f.stage == "sync" {
		return injected
	}
	return f.tempFile.Sync()
}
func (f *wrappedTemp) Close() error {
	err := f.tempFile.Close()
	if f.stage == "close" {
		return injected
	}
	return err
}
func TestActualFilePrepublicationFailuresPreserveOwnIntentAndExactRetry(t *testing.T) {
	for _, stage := range []string{"create", "write", "short", "sync", "close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			j, l, now, _ := prepared(t)
			before, err := os.ReadFile(j.path)
			if err != nil {
				t.Fatal(err)
			}
			saved := *j.data
			original := j.fs
			j.fs.create = func(dir string) (tempFile, error) {
				if stage == "create" {
					return nil, injected
				}
				f, err := original.create(dir)
				if err != nil {
					return nil, err
				}
				return &wrappedTemp{f, stage}, nil
			}
			if stage == "rename" {
				j.fs.rename = func(string, string) error { return injected }
			}
			if err := j.RecordCallback(l.AttemptID, l.State, code(), now); err == nil {
				t.Fatal("injected failure reported success")
			}
			after, err := os.ReadFile(j.path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("prepublication changed file", err)
			}
			if j.fault || j.data.Phase != "prepared" || j.data.Credential != saved.Credential || j.data.Verifier != saved.Verifier {
				t.Fatal("prepublication changed live intent")
			}
			entries, err := os.ReadDir(j.owner.Parent())
			if err != nil || len(entries) != 2 {
				t.Fatal("temp leaked", err)
			}
			j.fs = original
			if err := j.RecordCallback(l.AttemptID, l.State, code(), now); err != nil {
				t.Fatal("exact retry failed", err)
			}
		})
	}
}
func TestLocalPublicationUncertaintyDistinctFromRemoteConsumption(t *testing.T) {
	for _, stage := range []string{"after-rename", "dirsync"} {
		t.Run(stage, func(t *testing.T) {
			j, l, now, dir := prepared(t)
			original := j.fs
			if stage == "after-rename" {
				j.fs.rename = func(a, b string) error {
					if err := original.rename(a, b); err != nil {
						return err
					}
					return injected
				}
			} else {
				j.fs.syncDir = func(string) error { return injected }
			}
			if err := j.RecordCallback(l.AttemptID, l.State, code(), now); !errors.Is(err, ErrUncertain) {
				t.Fatal("local uncertain publish not classified", err)
			}
			if _, err := j.Status(); !errors.Is(err, ErrUncertain) {
				t.Fatal("uncertain object still usable", err)
			}
			if err := j.MarkExchangeStarted(now); !errors.Is(err, ErrUncertain) {
				t.Fatal("uncertain object advanced", err)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if phase, err := reopened.Status(); err != nil || phase != "callback-recorded" {
				t.Fatal("local recovery confused with remote may-have-started", err)
			}
		})
	}
}
func TestSecondOwnerCloseAndRace(t *testing.T) {
	j, l, now, dir := prepared(t)
	if other, err := Open(dir); err == nil {
		other.Close()
		t.Fatal("second owner admitted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); j.Status(); j.RecordCallback(l.AttemptID, l.State, code(), now) }()
	}
	wg.Wait()
	if phase, err := j.Status(); err != nil || phase != "callback-recorded" {
		t.Fatal("concurrent callback outcome wrong", err)
	}
	j.Close()
	j.Close()
	if _, err := j.Status(); !errors.Is(err, ErrClosed) {
		t.Fatal("closed status usable", err)
	}
}
func TestMalformedSavedIntentRefusedWithoutContentRewrite(t *testing.T) {
	for _, kind := range []string{"unknown", "duplicate", "null", "credential", "challenge", "phase", "missing", "trailing", "too-large"} {
		t.Run(kind, func(t *testing.T) {
			j, _, _, dir := prepared(t)
			raw, err := os.ReadFile(j.path)
			if err != nil {
				t.Fatal(err)
			}
			j.Close()
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "unknown":
				m["attacker"] = true
			case "duplicate":
				raw = append([]byte(`{"version":1,`), raw[1:]...)
			case "null":
				m["code"] = nil
			case "credential":
				m["credential"] = code()
			case "challenge":
				m["verifier"] = code()
			case "phase":
				m["phase"] = "accepted"
			case "missing":
				delete(m, "exchange_json")
			case "trailing":
				raw = append(raw, []byte(` {}`)...)
			case "too-large":
				raw = bytes.Repeat([]byte(" "), 65537)
			}
			if kind != "duplicate" && kind != "trailing" && kind != "too-large" {
				raw, _ = json.Marshal(m)
			}
			if err := os.WriteFile(j.path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if reopened, err := Open(dir); err == nil {
				reopened.Close()
				t.Fatal("corrupt journal accepted")
			}
			after, err := os.ReadFile(j.path)
			if err != nil || !bytes.Equal(raw, after) {
				t.Fatal("corrupt journal content rewritten", err)
			}
		})
	}
}
func TestFormattingDoesNotPrintNativeSecrets(t *testing.T) {
	j, _, _, _ := prepared(t)
	for _, s := range []string{fmt.Sprint(j), fmt.Sprintf("%#v", j), fmt.Sprint(*j), fmt.Sprintf("%#v", *j), fmt.Sprint(*j.data), fmt.Sprintf("%#v", *j.data)} {
		if bytes.Contains([]byte(s), []byte(j.data.Credential)) || bytes.Contains([]byte(s), []byte(j.data.Verifier)) {
			t.Fatal("formatter exposed secret")
		}
	}
}

var _ io.Writer = (*wrappedTemp)(nil)

func TestRestartPreparedNeverRestoresOldListener(t *testing.T) {
	j, l, now, dir := prepared(t)
	j.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if phase, err := reopened.Status(); err != nil || phase != "prepared-listener-not-owned" {
		t.Fatal("old listener implicitly restored", err)
	}
	if err := reopened.RecordCallback(l.AttemptID, l.State, code(), now); err == nil {
		t.Fatal("old callback accepted without listener ownership")
	}
	if err := reopened.MarkExchangeStarted(now); err == nil {
		t.Fatal("abandoned listener became network request")
	}
}

func TestBeginPublicationFailureDoesNotReturnLaunchOrRegenerateExisting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	original := j.fs
	j.fs.rename = func(string, string) error { return injected }
	now := time.Unix(1800000000, 0)
	mapping := contract.SourceMapping{WorkspaceID: "org", Issuer: "https://issuer.test", RegistrarOrigin: "https://registrar.test", Audience: "fixture", MappingRevision: "r1", NetworkID: "net"}
	launch, err := j.Begin(code(), "linux", "http://127.0.0.1:34567/mnet/enrollment/callback", fixturePolicy(t, "https://source.test", mapping), now, now.Add(time.Minute))
	if err == nil || launch != (contract.SourceLaunch{}) || j.data != nil {
		t.Fatal("failed Begin returned unpublished launch")
	}
	if _, err := os.Stat(j.path); !os.IsNotExist(err) {
		t.Fatal("prepublish failed Begin left state")
	}
	j.fs = original
	if _, err := j.Begin(code(), "linux", "http://127.0.0.1:34567/mnet/enrollment/callback", fixturePolicy(t, "https://source.test", mapping), now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
}
func TestJournalParentAndLockReplacementFreezePermanently(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows denies held lock rename; native execution required")
	}
	for _, kind := range []string{"parent", "lock"} {
		t.Run(kind, func(t *testing.T) {
			j, l, now, dir := prepared(t)
			path := j.owner.LockPath()
			if kind == "parent" {
				path = dir
			}
			if err := os.Rename(path, path+".old"); err != nil {
				t.Fatal(err)
			}
			if kind == "parent" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := j.Status(); !errors.Is(err, ErrUncertain) {
				t.Fatal("replacement not detected", err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path+".old", path); err != nil {
				t.Fatal(err)
			}
			if err := j.RecordCallback(l.AttemptID, l.State, code(), now); !errors.Is(err, ErrUncertain) {
				t.Fatal("restoring path resurrected authority", err)
			}
		})
	}
}
func TestJournalSubprocessDeathReleasesOwner(t *testing.T) {
	if dir := os.Getenv("MNET_PENDING_TEST_CHILD_DIR"); dir != "" {
		j, err := Open(dir)
		if err != nil {
			os.Exit(4)
		}
		defer j.Close()
		fmt.Println("owner-ready")
		time.Sleep(time.Minute)
		return
	}
	dir := filepath.Join(t.TempDir(), "private")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestJournalSubprocessDeathReleasesOwner$")
	cmd.Env = append(os.Environ(), "MNET_PENDING_TEST_CHILD_DIR="+dir)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(pipe); ready <- scanner.Scan() && scanner.Text() == "owner-ready" }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child owner unavailable")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child owner timeout")
	}
	if j, err := Open(dir); err == nil {
		j.Close()
		t.Fatal("second process admitted")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("dead process retained ownership", err)
	}
	reopened.Close()
}
func FuzzSavedIntentStrictDecoder(f *testing.F) {
	public := code()
	r := record{Version: 2, Phase: "prepared", SourceOrigin: "https://source.test", Authority: "https://multies.test", Allowed: []contract.SourceMapping{{WorkspaceID: "org", Issuer: "https://issuer.test", RegistrarOrigin: "https://registrar.test", Audience: "fixture", MappingRevision: "r1", NetworkID: "net"}}, Credential: public, Verifier: public, CreatedMS: 1800000000000, ExpiresMS: 1800000060000}
	r.Policy = config.LocalConfigV2{Version: 2, Revision: "r1", SourceOrigin: r.SourceOrigin, PrincipalAuthority: r.Authority, PrincipalNamespace: contract.PrincipalNamespace, IssuerLabel: "https://issuer.test", RegistrarOrigin: "https://registrar.test", Audience: "fixture", ServiceID: "fixture-service", Bindings: []config.LocalBindingV2{{WorkspaceID: "org", NetworkID: "net", PrincipalIDs: []string{"fixture-local-user"}}}}
	r.Launch = contract.SourceLaunch{Version: 1, Purpose: "mnet-enrollment", AttemptID: public, DevicePublicKey: public, CredentialDigest: hash(public), Challenge: hash(public), State: public, RedirectURI: "http://127.0.0.1:34567/mnet/enrollment/callback", Platform: "linux"}
	if !validRecord(r) {
		f.Fatal("public fuzz seed invalid")
	}
	raw, _ := json.Marshal(r)
	f.Add(raw)
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"version":1,"version":1}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 70000 {
			t.Skip()
		}
		var r record
		if strictRecord(raw, &r) {
			validRecord(r)
		}
	})
}

func TestConcurrentCloseAndCallbackLeavesWholeReopenableImage(t *testing.T) {
	j, l, now, dir := prepared(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				j.Close()
			} else {
				j.RecordCallback(l.AttemptID, l.State, code(), now)
			}
		}(i)
	}
	wg.Wait()
	if _, err := j.Status(); !errors.Is(err, ErrClosed) {
		t.Fatal("Close raced into usable object", err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("Close race left partial file", err)
	}
	defer reopened.Close()
	phase, err := reopened.Status()
	if err != nil || (phase != "callback-recorded" && phase != "prepared-listener-not-owned") {
		t.Fatal("Close race returned invalid phase", err)
	}
}

func TestCopiedJournalHandleSharesLifecycleAndPendingImage(t *testing.T) {
	j, l, now, _ := prepared(t)
	copyHandle := *j
	if err := copyHandle.RecordCallback(l.AttemptID, l.State, code(), now); err != nil {
		t.Fatal(err)
	}
	if phase, err := j.Status(); err != nil || phase != "callback-recorded" {
		t.Fatal("copied handle diverged image", err)
	}
	copyHandle.Close()
	if _, err := j.Status(); !errors.Is(err, ErrClosed) {
		t.Fatal("copied handle bypassed closed owner", err)
	}
}

func fixturePolicy(t *testing.T, origin string, m contract.SourceMapping) *config.LocalPolicyV2 {
	t.Helper()
	p, err := config.ValidateLocalV2(config.LocalConfigV2{Version: 2, Revision: m.MappingRevision, SourceOrigin: origin, PrincipalAuthority: "https://multies.test", PrincipalNamespace: contract.PrincipalNamespace, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, ServiceID: "fixture-service", Bindings: []config.LocalBindingV2{{WorkspaceID: m.WorkspaceID, NetworkID: m.NetworkID, PrincipalIDs: []string{"fixture-local-user"}}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func savedPolicy(t *testing.T, j *Journal) *config.LocalPolicyV2 {
	t.Helper()
	p, err := config.ValidateLocalV2(j.data.Policy)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func savedConfig(t *testing.T, j *Journal) SourceConfig {
	t.Helper()
	c, ok := SourceConfigFor(savedPolicy(t, j))
	if !ok {
		t.Fatal("invalid fixture config")
	}
	return c
}
