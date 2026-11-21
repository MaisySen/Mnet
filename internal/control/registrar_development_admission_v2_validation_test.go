//go:build mnet_enrollment_development || multies_native_fixture

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestAdmissionV2MalformedLedgerImmutableRefusal(t *testing.T) {
	d, f := newV2WorkflowFixture(t, "https://127.0.0.1:4567")
	o, e := d.store.registrarWorkflowLocked()
	if e != nil {
		t.Fatal(e)
	}
	if e = d.store.admissionPolicy.reserve(f.request, o); e != nil {
		t.Fatal(e)
	}
	path, root := d.store.path, d.root
	original, _ := os.ReadFile(path)
	d.Close()
	for _, name := range []string{"missing-total", "wrong-total", "negative-total", "null-entries", "null-entry", "unknown", "case", "duplicate", "fraction", "overflow", "entry-unknown", "entry-missing", "entry-phase", "premature-principal", "premature-observation", "premature-challenge", "premature-receipt", "oversize-principal", "trailing", "oversize-image"} {
		t.Run(name, func(t *testing.T) {
			var image map[string]any
			json.Unmarshal(original[len(developmentV2Header):], &image)
			l := image["admission_ledger"].(map[string]any)
			entries := l["entries"].(map[string]any)
			entry := entries[f.request.Launch.AttemptID].(map[string]any)
			switch name {
			case "missing-total":
				delete(l, "source_total")
			case "wrong-total":
				l["source_total"] = 0
			case "negative-total":
				l["source_total"] = -1
			case "null-entries":
				l["entries"] = nil
			case "null-entry":
				entries[f.request.Launch.AttemptID] = nil
			case "unknown":
				l["extra"] = 0
			case "case":
				l["Version"] = l["version"]
				delete(l, "version")
			case "fraction":
				l["source_total"] = 1.5
			case "entry-unknown":
				entry["extra"] = 0
			case "entry-missing":
				delete(entry, "source_debit")
			case "entry-phase":
				entry["phase"] = "resume"
			case "premature-principal":
				entry["principal_namespace"] = "multies-local-user-v1"
			case "premature-observation":
				entry["checked_at_ms"] = 1
			case "premature-challenge":
				entry["challenge_expires_at_ms"] = 1
			case "premature-receipt":
				entry["receipt_digest"] = f.request.Launch.CredentialDigest
			case "oversize-principal":
				entry["principal_id"] = strings.Repeat("<", 4096)
			}
			payload, _ := json.Marshal(image)
			if name == "duplicate" {
				payload = bytes.Replace(payload, []byte(`"source_total":1`), []byte(`"source_total":1,"source_total":1`), 1)
			}
			if name == "overflow" {
				payload = bytes.Replace(payload, []byte(`"source_total":1`), []byte(`"source_total":9223372036854775808`), 1)
			}
			raw := append([]byte(developmentV2Header), payload...)
			if name == "trailing" {
				raw = append(raw, []byte(`{}`)...)
			}
			if name == "oversize-image" {
				raw = append(raw, bytes.Repeat([]byte(" "), developmentV2ImageCap)...)
			}
			os.WriteFile(path, raw, 0600)
			if got, err := OpenDisposableRegistrarV2(root); err == nil {
				got.Close()
				t.Fatal("malformed adopted")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatal("refusal rewrote image")
			}
		})
	}
	os.WriteFile(path, original, 0600)
	reopened, e := OpenDisposableRegistrarV2(root)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	inspection, e := reopened.InspectAdmission()
	if e != nil || inspection.SourceWorkSpent != 1 || inspection.StrandedPrecommit != 1 || inspection.CommittedHistory != 0 {
		t.Fatal("spent stranded inspection", inspection, e)
	}
	if _, e = reopened.store.beginRegistrarEnrollment(context.Background(), f.request, f.secret); e == nil {
		t.Fatal("stranded attempt resumed")
	}
	changed := f.request
	changed.Ticket = "changed"
	if _, e = reopened.store.beginRegistrarEnrollment(context.Background(), changed, f.secret); e == nil {
		t.Fatal("stranded conflict allowed")
	}
	if reopened.store.data.Admission.SourceTotal != 1 {
		t.Fatal("stranded operation refunded")
	}
	if _, e = (developmentV2Codec{}).encode(bytes.Repeat([]byte("{"), developmentV2ImageCap)); e == nil {
		t.Fatal("oversize encoded")
	}
}

func TestAdmissionV2ReaderMarkerPayloadMatrix(t *testing.T) {
	legacy, e := Open(t.TempDir() + "/legacy.json")
	if e != nil {
		t.Fatal(e)
	}
	legacy.CreateNetwork("legacy")
	legacyRaw, _ := os.ReadFile(legacy.path)
	legacy.Close()
	v1, e := NewDisposableRegistrar(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	v1Raw, _ := os.ReadFile(v1.store.path)
	v1.Close()
	v2, e := NewDisposableRegistrarV2(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	v2Raw, _ := os.ReadFile(v2.store.path)
	root, path := v2.root, v2.store.path
	v2.Close()
	for markerName, marker := range map[string]string{"V1": developmentRootMarker, "V2": developmentV2Marker} {
		for imageName, image := range map[string][]byte{"legacy": legacyRaw, "V1": v1Raw, "V2": v2Raw} {
			t.Run(markerName+"/"+imageName, func(t *testing.T) {
				os.WriteFile(root+"/root-kind", []byte(marker), 0600)
				os.WriteFile(path, image, 0600)
				a, err := OpenDisposableRegistrar(root)
				if a != nil {
					a.Close()
				}
				if (err == nil) != (markerName == "V1" && imageName == "V1") {
					t.Fatal("V1 reader mismatch", err)
				}
				b, err := OpenDisposableRegistrarV2(root)
				if b != nil {
					b.Close()
				}
				if (err == nil) != (markerName == "V2" && imageName == "V2") {
					t.Fatal("V2 reader mismatch", err)
				}
				c, err := Open(path)
				if c != nil {
					c.Close()
				}
				if (err == nil) != (imageName == "legacy") {
					t.Fatal("legacy envelope mismatch", err)
				}
				after, _ := os.ReadFile(path)
				if !bytes.Equal(image, after) {
					t.Fatal("reader rewrote")
				}
			})
		}
	}
}

func TestAdmissionV2ConfigurePublicationFailureNeverReady(t *testing.T) {
	for _, stage := range []string{"create-temp", "rename", "sync-directory"} {
		t.Run(stage, func(t *testing.T) {
			d, e := NewDisposableRegistrarV2(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			sources, workflow, origin := developmentConfigurationFixture(t, d)
			before, _ := os.ReadFile(d.store.path)
			fs := d.store.fs
			if stage == "create-temp" {
				d.store.fs.createTemp = func(string) (storeTemp, error) { return nil, errInjectedStoreIO }
			}
			if stage == "rename" {
				d.store.fs.rename = func(string, string) error { return errInjectedStoreIO }
			}
			if stage == "sync-directory" {
				d.store.fs.syncDir = func(string) error { return errInjectedStoreIO }
			}
			if e = d.Configure(sources, workflow, origin); e == nil {
				t.Fatal("failed pin returned success")
			}
			d.store.fs = fs
			if _, e = d.Handler(); e == nil {
				t.Fatal("failed Configure served")
			}
			path, root := d.store.path, d.root
			d.Close()
			after, _ := os.ReadFile(path)
			reopened, e := OpenDisposableRegistrarV2(root)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			if stage != "sync-directory" {
				if !bytes.Equal(before, after) {
					t.Fatal("prepublication pin changed image")
				}
				if _, e = reopened.Handler(); e == nil {
					t.Fatal("unconfigured reopen adopted sidecars")
				}
				if e = reopened.Configure(sources, workflow, origin); e == nil {
					t.Fatal("partial root reconfigured")
				}
			} else {
				if bytes.Equal(before, after) {
					t.Fatal("postpublication pin absent")
				}
				if reopened.store.data.Admission == nil {
					t.Fatal("published zero ledger missing")
				}
			}
		})
	}
}
