package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	pending "fintex.au/mnet/internal/enrollmentpending"
)

func nativeCallbackTarget(l contract.SourceLaunch) string {
	return l.RedirectURI + "?" + url.Values{"state": {l.State}, "attemptId": {l.AttemptID}, "code": {base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))}}.Encode()
}
func invokeNativeCallback(t *testing.T, l contract.SourceLaunch) int {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	r, err := client.Get(nativeCallbackTarget(l))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	io.Copy(io.Discard, r.Body)
	return r.StatusCode
}
func TestNativePreparationUsesActualOwnedKeyFullPolicyAndFixedJournal(t *testing.T) {
	d, policyPath, policyBytes := enrollmentContextFixture(t)
	statePath := d.stateOwner.path
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	l, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	p := d.enrollmentPreparation
	key, err := base64.RawURLEncoding.Strict().DecodeString(l.DevicePublicKey)
	if err != nil || base64.StdEncoding.EncodeToString(key) != d.stateOwner.saved.PublicKey {
		t.Fatal("launch key not owned native key bytes")
	}
	if l != p.launch || p.context.owner != d.stateOwner {
		t.Fatal("preparation detached from owner")
	}
	raw, err := os.ReadFile(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending", "enrollment-pending-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Policy json.RawMessage       `json:"policy"`
		Launch contract.SourceLaunch `json:"launch"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved.Policy, policyBytes) || saved.Launch != l {
		t.Fatal("journal didn't pin actual policy/launch")
	}
	if _, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute)); !errors.Is(err, pending.ErrPending) {
		t.Fatal("preparation silently replaced", err)
	}
	if status := invokeNativeCallback(t, l); status != 200 {
		t.Fatal("actual owned callback rejected", status)
	}
	if phase, _ := p.journal.Status(); phase != "callback-recorded" {
		t.Fatal("callback not persisted")
	}
	after, _ := os.ReadFile(statePath)
	policyAfter, _ := os.ReadFile(policyPath)
	if !bytes.Equal(before, after) || !bytes.Equal(policyBytes, policyAfter) || d.cc != nil || d.dev != nil || d.state.NodeID != "" || d.state.NodeToken != "" {
		t.Fatal("preparation changed identity/config/account/tunnel")
	}
}
func TestNativePreparationMissingPolicyOrKeyMakesNoJournal(t *testing.T) {
	for _, kind := range []string{"policy", "key", "malformed", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			d, path, _ := enrollmentContextFixture(t)
			switch kind {
			case "policy":
				os.Remove(path)
			case "key":
				os.Remove(d.stateOwner.path)
			case "malformed":
				os.WriteFile(path, []byte(`{}`), 0600)
			}
			expires := time.Now().Add(time.Minute)
			if kind == "deadline" {
				expires = time.Now().Add(16 * time.Minute)
			}
			l, err := d.prepareNativeEnrollment(expires)
			if err == nil || l != (contract.SourceLaunch{}) || d.enrollmentPreparation != nil {
				t.Fatal("invalid native context prepared launch", err)
			}
			// Deadline is checked by callback before Begin; Open may create dedicated
			// private directory/lock but no intent file, never a key or replacement.
			if _, err := os.Stat(filepath.Join(d.stateOwner.lease.Parent(), "enrollment-pending", "enrollment-pending-v1.json")); !os.IsNotExist(err) {
				t.Fatal("invalid preparation saved intent")
			}
		})
	}
}
func TestNativePreparationPolicyOrStateChangesRefuseActualCallback(t *testing.T) {
	for _, kind := range []string{"policy-change", "policy-missing", "state-change", "working-state", "fault"} {
		t.Run(kind, func(t *testing.T) {
			d, path, raw := enrollmentContextFixture(t)
			l, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "policy-change":
				os.WriteFile(path, bytes.Replace(raw, []byte("local-user"), []byte("other-user"), 1), 0600)
			case "policy-missing":
				os.Remove(path)
			case "state-change":
				raw, _ := os.ReadFile(d.stateOwner.path)
				os.WriteFile(d.stateOwner.path, append(raw, ' '), 0600)
			case "working-state":
				d.state.NodeName = "changed"
			case "fault":
				d.stateOwner.fault = true
			}
			if status := invokeNativeCallback(t, l); status != 400 {
				t.Fatal("changed native authority context accepted", status)
			}
			if phase, _ := d.enrollmentPreparation.journal.Status(); phase != "prepared" {
				t.Fatal("refused callback consumed intent")
			}
		})
	}
}
func TestNativePreparationCloseReleasesJournalAndSocketWithoutRecoveringOldLaunch(t *testing.T) {
	d, _, _ := enrollmentContextFixture(t)
	dir := d.stateOwner.lease.Parent()
	l, err := d.prepareNativeEnrollment(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	p := d.enrollmentPreparation
	d.Close()
	if _, err := p.journal.Status(); !errors.Is(err, pending.ErrClosed) {
		t.Fatal("daemon Close retained journal", err)
	}
	u, _ := url.Parse(l.RedirectURI)
	if conn, err := net.DialTimeout("tcp", u.Host, time.Second); err == nil {
		conn.Close()
		t.Fatal("daemon Close retained listener")
	}
	reopened, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.enrollmentPreparation != nil {
		t.Fatal("New auto resumed enrollment")
	}
	if _, err := reopened.prepareNativeEnrollment(time.Now().Add(time.Minute)); !errors.Is(err, pending.ErrPending) {
		t.Fatal("restart silently renewed pending intent", err)
	}
	if reopened.enrollmentPreparation != nil {
		t.Fatal("failed recovery installed preparation")
	}
	if strings.Contains(p.String(), l.State) {
		t.Fatal("private formatter leaked launch")
	}
}
