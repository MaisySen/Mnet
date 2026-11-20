package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fintex.au/mnet/internal/localapi"
)

func TestPrepareIdentityDoesNotEnrollOrStartNetwork(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Config{StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	keys := make(chan string, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, e := d.PrepareIdentity()
			if e != nil {
				t.Error(e)
			}
			keys <- key
		}()
	}
	wg.Wait()
	close(keys)
	var first string
	for key := range keys {
		if first == "" {
			first = key
		}
		if key != first || key == "" {
			t.Fatal("identity replaced or missing")
		}
	}
	state := d.State()
	if d.Status().Status != "down" || state.Server != "" || state.Network != "" || state.NodeID != "" || state.NodeToken != "" || state.AuthKey != "" || d.dev != nil {
		t.Fatal("identity preparation started or enrolled network")
	}
	info, err := os.Stat(filepath.Join(dir, "state.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("identity not durable/private", err)
	}
	reloaded, err := LoadState(dir)
	if err != nil || reloaded.PublicKey != first {
		t.Fatal("identity not durable", err)
	}
}
func TestPrepareIdentityHTTPRequiresLocalCapabilityAndNoParameters(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	h := d.localAPIHandler("private-fixture-token")
	call := func(method, body, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://mnetd/localapi/identity", strings.NewReader(body))
		if token != "" {
			r.Header.Set(localapi.TokenHeader, token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if call("POST", "", "", "").Code != 401 || call("POST", "", "private-fixture-token", "https://example.test").Code != 403 {
		t.Fatal("unauthorized identity request accepted")
	}
	if d.State().PrivateKey != "" {
		t.Fatal("rejected request created identity")
	}
	if call("GET", "", "private-fixture-token", "").Code != 405 || call("POST", "{}", "private-fixture-token", "").Code != 400 {
		t.Fatal("parameters accepted")
	}
	w := call("POST", "", "private-fixture-token", "")
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	var reply map[string]string
	if json.Unmarshal(w.Body.Bytes(), &reply) != nil || len(reply) != 1 || reply["public_key"] == "" {
		t.Fatal("identity response invalid")
	}
	if bytes.Contains(w.Body.Bytes(), []byte(d.State().PrivateKey)) {
		t.Fatal("private key leaked")
	}
}
func TestPrepareIdentityRefusesBrokenStateAndClosedDaemon(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	d.state.PrivateKey = "broken"
	if _, err = d.PrepareIdentity(); err == nil || d.state.PrivateKey != "broken" {
		t.Fatal("broken identity replaced")
	}
	d.state.PrivateKey = ""
	d.Close()
	if _, err = d.PrepareIdentity(); err == nil {
		t.Fatal("closed daemon prepared identity")
	}
}
