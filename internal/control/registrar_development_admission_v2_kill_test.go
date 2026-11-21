//go:build (mnet_enrollment_development || multies_native_fixture) && !windows

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	proof "fintex.au/mnet/internal/enrollmentproof"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestAdmissionV2ActualKillChild(t *testing.T) {
	dir := os.Getenv("MNET_TEST_ADMISSION_KILL_DIR")
	if dir == "" {
		t.Skip("childonly")
	}
	target, _ := strconv.Atoi(os.Getenv("MNET_TEST_ADMISSION_KILL_STAGE"))
	boundary := os.Getenv("MNET_TEST_ADMISSION_KILL_BOUNDARY")
	calls, dh := 0, 0
	var active http.HandlerFunc
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; active(w, r) }))
	defer source.Close()
	cert := filepath.Join(dir, "roots.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
	t.Setenv("SSL_CERT_FILE", cert)
	t.Setenv("SSL_CERT_DIR", filepath.Join(dir, "no-roots"))
	// Root survives external kill; no t.TempDir ancestor may be cleaned by parent
	// before the actual owned-image assertion.
	d, e := NewDisposableRegistrarV2(dir)
	if e != nil {
		t.Fatal(e)
	}
	f := newWorkflowFixture(t, source.URL)
	raw, _ := os.ReadFile(f.s.path + ".registrar-sources-v1.json")
	f.s.Close()
	var sources registrarSourceConfig
	json.Unmarshal(raw, &sources)
	origin := "https://127.0.0.1:4568"
	f.request.Mapping.NetworkID = d.network
	f.request.Mapping.RegistrarOrigin = origin
	f.observed.Mapping = f.request.Mapping
	f.observed.DescriptorDigest, _ = contract.ServiceReserveDescriptorDigest(f.request, f.policy.Policy.ServiceID)
	f.policy.Policy.Bindings[0].NetworkID = d.network
	f.policy.Policy.RegistrarOrigin = origin
	sources.Sources[0].RegistrarOrigin = origin
	sources.Sources[0].Allowed = []contract.SourceMapping{f.request.Mapping}
	a, _ := json.Marshal(sources)
	b, _ := json.Marshal(f.policy)
	if e = d.Configure(a, b, origin); e != nil {
		t.Fatal(e)
	}
	f.s = d.store
	active = f.handler(t)
	d.store.registrarProofFactory = func(b contract.Binding, start, end time.Time, duration time.Duration) (*proof.Server, contract.Challenge, error) {
		dh++
		return proof.NewServer(b, start, end, duration)
	}
	signal := func() {
		raw, _ := json.Marshal(map[string]any{"root": d.root, "path": d.store.path, "sourceCalls": calls, "dhAllocations": dh})
		if e := os.WriteFile(filepath.Join(dir, "boundary.tmp"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		// Publish the test IPC signal only after the complete metadata exists.
		// Otherwise the external killer can observe an empty/partial file and kill
		// the child before its metadata write finishes, unrelated to Store I/O.
		if e := os.Rename(filepath.Join(dir, "boundary.tmp"), filepath.Join(dir, "boundary.json")); e != nil {
			t.Fatal(e)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	real := d.store.fs
	publication := 0
	d.store.fs.createTemp = func(parent string) (storeTemp, error) {
		publication++
		tmp, e := real.createTemp(parent)
		if e != nil {
			return nil, e
		}
		if publication == target {
			before, _ := os.ReadFile(d.store.path)
			os.WriteFile(filepath.Join(dir, "before-image"), before, 0600)
			if boundary == "after-tempcreate" {
				signal()
			}
			return developmentKillTemp{tmp, boundary, signal}, nil
		}
		return tmp, nil
	}
	d.store.fs.rename = func(a, b string) error {
		if publication == target && boundary == "before-rename" {
			signal()
		}
		e := real.rename(a, b)
		if publication == target && boundary == "after-rename" {
			signal()
		}
		return e
	}
	d.store.fs.syncDir = func(path string) error {
		e := real.syncDir(path)
		if publication == target && boundary == "after-dirsync" {
			signal()
		}
		return e
	}
	c, e := d.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret)
	if e != nil {
		t.Fatal(e)
	}
	if target == 4 {
		_, response := f.respond(t, c)
		if _, e = d.store.submitRegistrarEnrollment(context.Background(), c.Binding.AttemptID, c.ID, f.secret, response); e != nil {
			t.Fatal(e)
		}
	}
	t.Fatal("killboundarymissed")
}
func TestAdmissionV2ActualKillFourStages(t *testing.T) {
	for stage := 1; stage <= 4; stage++ {
		for _, boundary := range []string{"after-tempcreate", "after-write", "after-filesync", "before-rename", "after-rename", "after-dirsync"} {
			t.Run(fmt.Sprintf("stage%d-%s", stage, boundary), func(t *testing.T) {
				dir := t.TempDir()
				cmd := exec.Command(os.Args[0], "-test.run=^TestAdmissionV2ActualKillChild$", "-test.timeout=30s")
				cmd.Env = append(os.Environ(), "MNET_TEST_ADMISSION_KILL_DIR="+dir, fmt.Sprintf("MNET_TEST_ADMISSION_KILL_STAGE=%d", stage), "MNET_TEST_ADMISSION_KILL_BOUNDARY="+boundary)
				var output bytes.Buffer
				cmd.Stdout = &output
				cmd.Stderr = &output
				if e := cmd.Start(); e != nil {
					t.Fatal(e)
				}
				defer func() {
					if cmd.ProcessState == nil {
						cmd.Process.Kill()
						cmd.Wait()
					}
				}()
				deadline := time.Now().Add(12 * time.Second)
				var raw []byte
				for {
					var e error
					raw, e = os.ReadFile(filepath.Join(dir, "boundary.json"))
					if e == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("childboundarytimeout")
					}
					time.Sleep(2 * time.Millisecond)
				}
				if e := cmd.Process.Signal(syscall.SIGKILL); e != nil {
					t.Fatal(e)
				}
				if e := cmd.Wait(); e == nil {
					t.Fatal("killreturnedsuccess")
				}
				status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
				if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
					t.Fatalf("notSIGKILL %s", output.String())
				}
				var meta struct {
					Root        string `json:"root"`
					Path        string `json:"path"`
					SourceCalls int    `json:"sourceCalls"`
					DH          int    `json:"dhAllocations"`
				}
				if json.Unmarshal(raw, &meta) != nil {
					t.Fatal("boundarymeta")
				}
				if stage == 1 && (meta.SourceCalls != 0 || meta.DH != 0) {
					t.Fatal("sourceorDHbeforestage1")
				}
				before, _ := os.ReadFile(filepath.Join(dir, "before-image"))
				image, e := os.ReadFile(meta.Path)
				if e != nil {
					t.Fatal(e)
				}
				post := boundary == "after-rename" || boundary == "after-dirsync"
				if !post && !bytes.Equal(before, image) {
					t.Fatal("killedprepubchangedimage")
				}
				d, e := OpenDisposableRegistrarV2(meta.Root)
				if e != nil {
					t.Fatal("killedwholeimageunreadable", e)
				}
				defer d.Close()
				after, _ := os.ReadFile(meta.Path)
				if !bytes.Equal(image, after) {
					t.Fatal("killrecoveryrewrite")
				}
				l := d.store.data.Admission
				if stage > 1 || post {
					if l.SourceTotal != 1 {
						t.Fatal("killrefundeddebit")
					}
				} else if l.SourceTotal != 0 {
					t.Fatal("predebitkillcharged")
				}
				wanted := stage - 1
				if post {
					wanted = stage
				}
				if wanted > 0 {
					var entry *registrarAdmissionEntry
					for _, v := range l.Entries {
						entry = v
					}
					phases := map[int]string{1: "source-work-reserved", 2: "source-observed-principal-debited", 3: "challenge-issued", 4: "committed"}
					if entry == nil || entry.Phase != phases[wanted] {
						t.Fatal("partialstageimage")
					}
				}
				if wanted == 4 {
					if len(d.store.data.Nodes) != 1 || len(d.store.data.Registrar.Records) != 1 || len(d.store.data.Registrar.ConsumedProofs) != 1 {
						t.Fatal("partialcommittedcandidate")
					}
				}
				t.Logf("actualSIGKILL stage=%d boundary=%s source=%d DH=%d noRewrite=true", stage, boundary, meta.SourceCalls, meta.DH)
			})
		}
	}
}
