//go:build mnet_enrollment_development || multies_native_fixture

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newV2WorkflowFixture(t *testing.T, source string) (*DisposableRegistrar, *workflowFixture) {
	t.Helper()
	d, e := NewDisposableRegistrarV2(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	f := newWorkflowFixture(t, source)
	defer f.s.Close()
	raw, _ := os.ReadFile(f.s.path + ".registrar-sources-v1.json")
	var c registrarSourceConfig
	json.Unmarshal(raw, &c)
	c.MaxAttempts = 8
	origin := "https://127.0.0.1:4568"
	f.request.Mapping.NetworkID = d.network
	f.request.Mapping.RegistrarOrigin = origin
	f.observed.Mapping = f.request.Mapping
	f.policy.Policy.Bindings[0].NetworkID = d.network
	f.policy.Policy.Bindings[0].PrincipalIDs = append(f.policy.Policy.Bindings[0].PrincipalIDs, "second-principal")
	f.policy.Policy.RegistrarOrigin = origin
	c.Sources[0].RegistrarOrigin = origin
	c.Sources[0].Allowed = []contract.SourceMapping{f.request.Mapping}
	f.observed.DescriptorDigest, _ = contract.ServiceReserveDescriptorDigest(f.request, f.policy.Policy.ServiceID)
	sources, _ := json.Marshal(c)
	workflow, _ := json.Marshal(f.policy)
	if e = d.Configure(sources, workflow, origin); e != nil {
		t.Fatal(e)
	}
	f.s = d.store
	return d, f
}
func TestAdmissionV2FreshTLSChild(t *testing.T) {
	if os.Getenv("MNET_TEST_ADMISSION_V2_CHILD") == "1" {
		admissionV2Flow(t)
		admissionV2FaultStages(t)
		admissionV2DelayedChallenge(t)
		admissionV2PrincipalFour(t)
		admissionV2ConcurrentAndUnknown(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAdmissionV2FreshTLSChild$", "-test.v", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), "MNET_TEST_ADMISSION_V2_CHILD=1")
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("admission child %v\n%s", e, out)
	}
}
func admissionV2Flow(t *testing.T) {
	var active http.HandlerFunc
	calls := 0
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; active(w, r) }))
	defer source.Close()
	cert := filepath.Join(t.TempDir(), "roots.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	d, f := newV2WorkflowFixture(t, source.URL)
	defer func() { d.Close() }()
	active = f.handler(t)
	c, e := d.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
	if e != nil {
		t.Fatal("V2 begin", e)
	}
	l := d.store.data.Admission
	if calls != 1 || l.SourceTotal != 1 || len(l.PrincipalTotals) != 1 || l.Entries[f.request.Launch.AttemptID].Phase != "challenge-issued" {
		t.Fatal("three-stage accounting missing")
	}
	native, response := f.respond(t, c)
	result, e := d.store.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response)
	if e != nil {
		t.Fatal("V2 commit", e)
	}
	if e = native.Confirm(result.Confirmation, time.Now()); e != nil {
		t.Fatal(e)
	}
	if d.store.data.Admission.Entries[f.request.Launch.AttemptID].Phase != "committed" {
		t.Fatal("terminal debit absent")
	}
	admissionV2CommittedCorrelation(t, d)
	before, _ := os.ReadFile(d.store.path)
	root := d.root
	d.Close()
	d, e = OpenDisposableRegistrarV2(root)
	if e != nil {
		t.Fatal("reopen", e)
	}
	after, _ := os.ReadFile(d.store.path)
	if !bytes.Equal(before, after) {
		t.Fatal("load rewrite")
	}
	if _, e = d.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret); e == nil {
		t.Fatal("committed begin replay granted")
	}
	if calls != 1 {
		t.Fatal("restart source replay")
	}
}
func TestAdmissionV2ConfigurePinAndThreeWayRefusal(t *testing.T) {
	d, e := NewDisposableRegistrarV2(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if _, e = d.Handler(); e == nil {
		t.Fatal("unpinned handler allowed")
	}
	if _, e = d.store.registrarWorkflowLocked(); e == nil {
		t.Fatal("unpinned workflow allowed")
	}
	sources, workflow, origin := developmentConfigurationFixture(t, d)
	if e = d.Configure(sources, workflow, origin); e != nil {
		t.Fatal(e)
	}
	path := d.store.path
	before, _ := os.ReadFile(path)
	root := d.root
	d.Close()
	if old, e := Open(path); e == nil {
		old.Close()
		t.Fatal("legacy adoptedV2")
	}
	if old, e := OpenDisposableRegistrar(root); e == nil {
		old.Close()
		t.Fatal("V1 adoptedV2")
	}
	d, e = OpenDisposableRegistrarV2(root)
	if e != nil {
		t.Fatal(e)
	}
	os.Remove(path + ".registrar-workflow-v1.json")
	if e = d.Configure(sources, workflow, origin); e == nil {
		t.Fatal("reconfigureafterdelete allowed")
	}
	os.WriteFile(path+".registrar-workflow-v1.json", append(workflow, ' '), 0600)
	if _, e = d.Handler(); e == nil {
		t.Fatal("editedsidecar handler")
	}
	if _, e = d.store.registrarWorkflowLocked(); e == nil {
		t.Fatal("editedsidecar workflow")
	}
	d.Close()
	if other, e := OpenDisposableRegistrarV2(root); e == nil {
		other.Close()
		t.Fatal("edited reopen adopted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("refusal rewrote")
	}
	old, e := NewDisposableRegistrar(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	oldRoot := old.root
	old.Close()
	if v2, e := OpenDisposableRegistrarV2(oldRoot); e == nil {
		v2.Close()
		t.Fatal("V2 adoptedV1")
	}
}
func TestAdmissionV2ActualSchemaEncodingBounds(t *testing.T) {
	d := strings.Repeat("z", 43)
	entry := &registrarAdmissionEntry{Version: 2, Purpose: "mnet-initial-enrollment-v2", AttemptID: d, DescriptorDigest: d, RequestDigest: d, BootstrapDigest: d, ConfigFingerprint: d, SourceAuthorityDigest: d, Phase: "source-observed-principal-debited", SourceDebit: 1, PrincipalID: strings.Repeat("<", 256), PrincipalNamespace: contract.PrincipalNamespace, PrincipalTupleDigest: d, PrincipalDebit: 1, ObservationDigest: d, CheckedAtMS: math.MaxInt64, ReserveDeadlineMS: math.MaxInt64, ChallengeDigest: d, ChallengeExpiresAtMS: math.MaxInt64, AccessExpiresAtMS: math.MaxInt64, ReceiptID: d, ReceiptDigest: d}
	raw, _ := json.MarshalIndent(entry, "        ", "  ")
	if len(raw) > developmentLedgerEntryCap {
		t.Fatal("actual schema exceeds measuredentry")
	}
	l := registrarAdmissionLedger{Version: 2, ConfigFingerprint: d, SourceAuthorityDigest: d, SourceTotal: 8, PrincipalTotals: map[string]int{}, Entries: map[string]*registrarAdmissionEntry{}}
	for i := 0; i < 8; i++ {
		key := strings.Repeat(string(rune('a'+i)), 43)
		l.Entries[key] = entry
		l.PrincipalTotals[key] = 4
	}
	raw, _ = json.MarshalIndent(l, "  ", "  ")
	if len(raw) > developmentLedgerReserve {
		t.Fatal("actual ledger exceedsreserve")
	}
	t.Logf("actual ledger encoded bytes=%d reserved=%d", len(raw), developmentLedgerReserve)
}

func TestAdmissionV2CapacityThreeNotFourAndPermanentEight(t *testing.T) {
	d, f := newV2WorkflowFixture(t, "https://127.0.0.1:4567")
	defer d.Close()
	p := d.store.admissionPolicy.(*developmentAdmissionV2)
	o, e := d.store.registrarWorkflowLocked()
	if e != nil {
		t.Fatal(e)
	}
	for live := 0; live < 4; live++ {
		o.live = live
		e = p.capacity(o)
		if (live < 3) != (e == nil) {
			t.Fatalf("live=%d capacity=%v", live, e)
		}
	}
	o.live = 0
	for i := 0; i < 8; i++ {
		r := f.request
		r.Launch.AttemptID = registrarContractDigest(byte(10 + i))
		if e = p.reserve(r, o); e != nil {
			t.Fatalf("reserve%d %v", i, e)
		}
	}
	before, _ := os.ReadFile(d.store.path)
	r := f.request
	r.Launch.AttemptID = registrarContractDigest(90)
	if e = p.reserve(r, o); e == nil {
		t.Fatal("ninthunitrefunded")
	}
	after, _ := os.ReadFile(d.store.path)
	if !bytes.Equal(before, after) {
		t.Fatal("exhaustionrewrite")
	}
	root := d.root
	d.Close()
	d, e = OpenDisposableRegistrarV2(root)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	o, e = d.store.registrarWorkflowLocked()
	if e != nil {
		t.Fatal(e)
	}
	if e = d.store.admissionPolicy.reserve(r, o); e == nil {
		t.Fatal("reopenreset source total")
	}
	if d.store.data.Admission.SourceTotal != 8 {
		t.Fatal("spent total lost")
	}
}
func TestAdmissionV2StageOneFaultsZeroHTTPZeroDH(t *testing.T) {
	for _, stage := range []string{"create-temp", "write", "short-write", "sync-file", "close-file", "rename"} {
		t.Run(stage, func(t *testing.T) {
			calls := 0
			source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
			defer source.Close()
			d, f := newV2WorkflowFixture(t, source.URL)
			defer d.Close()
			allocations := 0
			d.store.registrarProofFactory = func(b contract.Binding, start, end time.Time, duration time.Duration) (*proof.Server, contract.Challenge, error) {
				allocations++
				return proof.NewServer(b, start, end, duration)
			}
			before, _ := os.ReadFile(d.store.path)
			real := d.store.fs
			d.store.fs.createTemp = func(dir string) (storeTemp, error) {
				if stage == "create-temp" {
					return nil, errInjectedStoreIO
				}
				tmp, e := real.createTemp(dir)
				if e != nil {
					return nil, e
				}
				return faultStoreTemp{tmp, stage}, nil
			}
			if stage == "rename" {
				d.store.fs.rename = func(string, string) error { return errInjectedStoreIO }
			}
			if _, e := d.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret); e == nil {
				t.Fatal("failurehidden")
			}
			if calls != 0 || allocations != 0 {
				t.Fatalf("source=%d DH=%d beforedebit", calls, allocations)
			}
			after, _ := os.ReadFile(d.store.path)
			if !bytes.Equal(before, after) || d.store.data.Admission.SourceTotal != 0 {
				t.Fatal("prepubfailurechangedimage")
			}
		})
	}
}

func admissionV2FaultStages(t *testing.T) {
	var active http.HandlerFunc
	calls := 0
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; active(w, r) }))
	defer source.Close()
	cert := filepath.Join(t.TempDir(), "roots.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	for target := 1; target <= 4; target++ {
		for _, fault := range []string{"create-temp", "write", "short-write", "sync-file", "close-file", "rename", "rename-after-publish", "sync-directory"} {
			t.Run(fmt.Sprintf("V2-stage%d-%s", target, fault), func(t *testing.T) {
				d, f := newV2WorkflowFixture(t, source.URL)
				defer d.Close()
				active = f.handler(t)
				beforeCalls := calls
				allocations := 0
				d.store.registrarProofFactory = func(b contract.Binding, start, end time.Time, duration time.Duration) (*proof.Server, contract.Challenge, error) {
					allocations++
					return proof.NewServer(b, start, end, duration)
				}
				real := d.store.fs
				publication := 0
				var previous []byte
				d.store.fs.createTemp = func(dir string) (storeTemp, error) {
					publication++
					if publication == target {
						previous, _ = os.ReadFile(d.store.path)
						if fault == "create-temp" {
							return nil, errInjectedStoreIO
						}
					}
					tmp, e := real.createTemp(dir)
					if e != nil {
						return nil, e
					}
					if publication == target {
						return faultStoreTemp{tmp, fault}, nil
					}
					return tmp, nil
				}
				d.store.fs.rename = func(a, b string) error {
					if publication == target && fault == "rename" {
						return errInjectedStoreIO
					}
					e := real.rename(a, b)
					if publication == target && fault == "rename-after-publish" && e == nil {
						return errInjectedStoreIO
					}
					return e
				}
				d.store.fs.syncDir = func(path string) error {
					e := real.syncDir(path)
					if publication == target && fault == "sync-directory" && e == nil {
						return errInjectedStoreIO
					}
					return e
				}
				c, e := d.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
				if target < 4 {
					if e == nil {
						t.Fatal("failedstage returnedchallenge")
					}
				} else {
					if e != nil {
						t.Fatal("beforecommit begin", e)
					}
					_, response := f.respond(t, c)
					if _, e = d.store.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); e == nil {
						t.Fatal("failedcommitreturnedconfirmation")
					}
				}
				if publication != target {
					t.Fatalf("wrongpublication%d wanted%d", publication, target)
				}
				if target == 1 && (calls != beforeCalls || allocations != 0) {
					t.Fatalf("stage1 source=%d DH=%d", calls-beforeCalls, allocations)
				}
				post := fault == "rename-after-publish" || fault == "sync-directory"
				raw, _ := os.ReadFile(d.store.path)
				if !post && !bytes.Equal(previous, raw) {
					t.Fatal("prepuboldimagechanged")
				}
				if post && d.store.Health() == nil {
					t.Fatal("postpublicationnotfrozen")
				}
				path, root := d.store.path, d.root
				d.Close()
				reopened, e := OpenDisposableRegistrarV2(root)
				if e != nil {
					t.Fatal("wholeimage reopen", e)
				}
				defer reopened.Close()
				after, _ := os.ReadFile(path)
				if !bytes.Equal(raw, after) {
					t.Fatal("loadrewrite")
				}
				total := reopened.store.data.Admission.SourceTotal
				if target == 1 && !post && total != 0 {
					t.Fatal("failedfirstdebitspent")
				}
				if (target > 1 || post) && total != 1 {
					t.Fatal("publisheddebitrefunded")
				}
				if target == 1 && !post {
					return
				} // No debit publication and zero external/crypto: no stranded operation exists.
				if _, e = reopened.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret); e == nil {
					t.Fatal("stranded replaygranted")
				}
				if calls-beforeCalls > 1 {
					t.Fatal("reopensourcesend")
				}
			})
		}
	}
}
func admissionV2DelayedChallenge(t *testing.T) {
	for _, stage := range []int{2, 3} {
		t.Run(fmt.Sprintf("delayed-stage-%d", stage), func(t *testing.T) { admissionV2DelayedPublication(t, stage) })
	}
}
func admissionV2DelayedPublication(t *testing.T, stage int) {
	var active http.HandlerFunc
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { active(w, r) }))
	defer source.Close()
	cert := filepath.Join(t.TempDir(), "roots.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	d, f := newV2WorkflowFixture(t, source.URL)
	defer d.Close()
	active = f.handler(t)
	// Narrow only this fixture's original interval before Configure pin. A NEW
	// fixture/root is required, never edit a pinned config to shorten/extend it.
	sources, _ := os.ReadFile(d.store.path + ".registrar-sources-v1.json")
	d.Close()
	fresh, e := NewDisposableRegistrarV2(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	var c registrarSourceConfig
	json.Unmarshal(sources, &c)
	f.request.Mapping.NetworkID = fresh.network
	f.observed.Mapping = f.request.Mapping
	f.observed.DescriptorDigest, _ = contract.ServiceReserveDescriptorDigest(f.request, f.policy.Policy.ServiceID)
	c.Sources[0].Allowed = []contract.SourceMapping{f.request.Mapping}
	f.policy.Policy.Bindings[0].NetworkID = fresh.network
	f.policy.ChallengeMS = 80
	f.policy.ClockUncertaintyMS = 1
	sources, _ = json.Marshal(c)
	workflow, _ := json.Marshal(f.policy)
	if e = fresh.Configure(sources, workflow, f.policy.Policy.RegistrarOrigin); e != nil {
		t.Fatal(e)
	}
	f.s = fresh.store
	real := fresh.store.fs
	publications := 0
	fresh.store.fs.createTemp = func(dir string) (storeTemp, error) { publications++; return real.createTemp(dir) }
	fresh.store.fs.syncDir = func(path string) error {
		e := real.syncDir(path)
		if publications == stage {
			time.Sleep(110 * time.Millisecond)
		}
		return e
	}
	allocations := 0
	fresh.store.registrarProofFactory = func(b contract.Binding, start, end time.Time, duration time.Duration) (*proof.Server, contract.Challenge, error) {
		allocations++
		return proof.NewServer(b, start, end, duration)
	}
	if _, e = fresh.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret); e == nil {
		t.Fatal("expired publishedchallenge returned")
	}
	phase := "challenge-issued"
	if stage == 2 {
		phase = "source-observed-principal-debited"
	}
	if allocations != stage-2 {
		t.Fatal("proof factory entered after stale observation", allocations)
	}
	if publications != stage || fresh.store.data.Admission.SourceTotal != 1 || fresh.store.data.Admission.Entries[f.request.Launch.AttemptID].Phase != phase {
		t.Fatal("spentpublicfingerprinthistorylost")
	}
}

func admissionV2PrincipalFour(t *testing.T) {
	var active http.HandlerFunc
	calls := 0
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; active(w, r) }))
	defer source.Close()
	cert := filepath.Join(t.TempDir(), "roots.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	// Fresh short-lived fixture policy, fixed before pin. Never mutate a pinned
	// lifetime just to wait out earlier live-slot reservations.
	d, f := newV2WorkflowFixture(t, source.URL)
	d.Close()
	sourcesRaw, _ := os.ReadFile(d.store.path + ".registrar-sources-v1.json")
	fresh, e := NewDisposableRegistrarV2(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	var sources registrarSourceConfig
	json.Unmarshal(sourcesRaw, &sources)
	f.request.Mapping.NetworkID = fresh.network
	f.observed.Mapping = f.request.Mapping
	f.policy.Policy.Bindings[0].NetworkID = fresh.network
	f.policy.ChallengeMS = 80
	sources.Sources[0].Allowed = []contract.SourceMapping{f.request.Mapping}
	sourcesRaw, _ = json.Marshal(sources)
	workflow, _ := json.Marshal(f.policy)
	if e = fresh.Configure(sourcesRaw, workflow, f.policy.Policy.RegistrarOrigin); e != nil {
		t.Fatal(e)
	}
	f.s = fresh.store
	active = f.handler(t)
	dh := 0
	fresh.store.registrarProofFactory = func(b contract.Binding, start, end time.Time, duration time.Duration) (*proof.Server, contract.Challenge, error) {
		dh++
		return proof.NewServer(b, start, end, duration)
	}
	for i := 0; i < 8; i++ {
		f.request.Launch.AttemptID = registrarContractDigest(byte(90 + i))
		f.request.HandoffID = registrarContractDigest(byte(110 + i))
		f.request.ReserveID = registrarContractDigest(byte(130 + i))
		f.observed.Launch = f.request.Launch
		f.observed.HandoffID = f.request.HandoffID
		f.observed.ReserveID = f.request.ReserveID
		f.observed.DescriptorDigest, _ = contract.ServiceReserveDescriptorDigest(f.request, f.policy.Policy.ServiceID)
		if i >= 5 {
			f.observed.Provenance.PrincipalID = "second-principal"
		}
		_, e = fresh.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
		if i == 4 {
			if e == nil || dh != 4 {
				t.Fatal("fifthprincipalworkallowed")
			}
		} else if e != nil {
			t.Fatalf("operation%d %v source=%d dh=%d entry=%+v RAM=%+v", i, e, calls, dh, fresh.store.data.Admission.Entries[f.request.Launch.AttemptID], fresh.store.registrarEnrollment.attempts[f.request.Launch.AttemptID])
		}
		time.Sleep(90 * time.Millisecond)
	}
	if calls != 8 || dh != 7 || fresh.store.data.Admission.SourceTotal != 8 || len(fresh.store.data.Admission.PrincipalTotals) != 2 {
		t.Fatalf("calls%d DH%d ledger%+v", calls, dh, fresh.store.data.Admission.PrincipalTotals)
	}
	for _, n := range fresh.store.data.Admission.PrincipalTotals {
		if n != 3 && n != 4 {
			t.Fatal("principal totalswrong")
		}
	}
	f.request.Launch.AttemptID = registrarContractDigest(200)
	if _, e = fresh.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret); e == nil || calls != 8 || dh != 7 {
		t.Fatal("ninthsourceworknotpermanentlyrefused")
	}
}

func admissionV2ConcurrentAndUnknown(t *testing.T) {
	var active http.HandlerFunc
	calls := 0
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; active(w, r) }))
	defer source.Close()
	cert := filepath.Join(t.TempDir(), "roots.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	d, f := newV2WorkflowFixture(t, source.URL)
	defer d.Close()
	active = f.handler(t)
	var wg sync.WaitGroup
	results := make(chan contract.Challenge, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := d.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
			if e == nil {
				results <- c
			}
		}()
	}
	wg.Wait()
	close(results)
	id := ""
	for c := range results {
		if id != "" && id != c.ID {
			t.Fatal("different challenge for one operation")
		}
		id = c.ID
	}
	if id == "" || calls != 1 || d.store.data.Admission.SourceTotal != 1 {
		t.Fatal("duplicate source work", calls)
	}
	changed := f.request
	changed.Ticket = registrarContractDigest(190)
	if _, e := d.store.beginRegistrarEnrollment(context.Background(), changed, f.secret); e == nil || calls != 1 {
		t.Fatal("changed digest permitted")
	}
	before, _ := os.ReadFile(d.store.path)
	sourcePath := d.store.path + ".registrar-sources-v1.json"
	original, _ := os.ReadFile(sourcePath)
	for _, field := range []string{"alias", "key"} {
		var config registrarSourceConfig
		json.Unmarshal(original, &config)
		if field == "alias" {
			config.Sources[0].ID = "another-alias"
		} else {
			config.Sources[0].KeyID = "another-key"
		}
		raw, _ := json.Marshal(config)
		os.WriteFile(sourcePath, raw, 0600)
		changed = f.request
		changed.Launch.AttemptID = registrarContractDigest(210)
		if _, e := d.store.beginRegistrarEnrollment(context.Background(), changed, f.secret); e == nil || calls != 1 {
			t.Fatal("edited namespace admitted")
		}
	}
	os.WriteFile(sourcePath, original, 0600)
	after, _ := os.ReadFile(d.store.path)
	if !bytes.Equal(before, after) {
		t.Fatal("config conflict reset ledger")
	}
	if other, e := OpenDisposableRegistrarV2(d.root); e == nil {
		other.Close()
		t.Fatal("second owner admitted")
	}
	bad, badFixture := newV2WorkflowFixture(t, source.URL)
	defer bad.Close()
	active = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) }
	if _, e := bad.store.beginRegistrarEnrollment(context.Background(), badFixture.request, badFixture.secret); e == nil {
		t.Fatal("invalid ticket accepted")
	}
	l := bad.store.data.Admission
	if l.SourceTotal != 1 || len(l.PrincipalTotals) != 0 || l.Entries[badFixture.request.Launch.AttemptID].PrincipalDebit != 0 {
		t.Fatal("invalid ticket charged victim principal")
	}
}

func admissionV2CommittedCorrelation(t *testing.T, d *DisposableRegistrar) {
	saved, _ := json.Marshal(d.store.data.Admission)
	for _, name := range []string{"tuple", "public-challenge", "reverse-record"} {
		var l registrarAdmissionLedger
		json.Unmarshal(saved, &l)
		d.store.data.Admission = &l
		for _, entry := range l.Entries {
			switch name {
			case "tuple":
				old := entry.PrincipalTupleDigest
				entry.PrincipalTupleDigest = registrarContractDigest(230)
				delete(l.PrincipalTotals, old)
				l.PrincipalTotals[entry.PrincipalTupleDigest] = 1
			case "public-challenge":
				entry.ChallengeDigest = registrarContractDigest(231)
			case "reverse-record":
				entry.Phase = "challenge-issued"
				entry.ReceiptID = ""
				entry.ReceiptDigest = ""
			}
		}
		if e := d.store.admissionPolicy.(*developmentAdmissionV2).validate(); e == nil {
			t.Fatal("bad committed correlation accepted", name)
		}
	}
	var restored registrarAdmissionLedger
	json.Unmarshal(saved, &restored)
	d.store.data.Admission = &restored
	if e := d.store.admissionPolicy.(*developmentAdmissionV2).validate(); e != nil {
		t.Fatal(e)
	}
}
