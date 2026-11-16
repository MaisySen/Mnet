package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminMembershipHTTPBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	network, err := st.CreateNetwork("fixture")
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := NewServer(st, nil, "https://control.invalid", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	app.sessions["valid"] = time.Now().Add(time.Hour)
	app.sessions["expired"] = time.Now().Add(-time.Minute)
	payload := `{"network_id":"` + network.ID + `","issuer":"https://principal.invalid","subject":"private-subject","allow":true}`
	request := func(method, body, cookie, media, origin, site string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "https://control.invalid/admin/api/memberships", strings.NewReader(body))
		req.Header.Set("Content-Type", media)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: cookie})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct {
		name, method, body, cookie, media, origin, site string
		want                                            int
	}{
		{"anonymous", "POST", payload, "", "application/json", "", "", 401},
		{"forged session", "POST", payload, "forged", "application/json", "", "", 401},
		{"expired session", "POST", payload, "expired", "application/json", "", "", 401},
		{"get forbidden", "GET", payload, "valid", "application/json", "", "", 405},
		{"form forbidden", "POST", payload, "valid", "text/plain", "", "", 415},
		{"cross origin", "POST", payload, "valid", "application/json", "https://other.invalid", "", 403},
		{"null origin", "POST", payload, "valid", "application/json", "null", "", 403},
		{"cross site", "POST", payload, "valid", "application/json", "", "cross-site", 403},
		{"sibling site", "POST", payload, "valid", "application/json", "", "same-site", 403},
		{"omitted decision", "POST", strings.Replace(payload, `,"allow":true`, "", 1), "valid", "application/json", "", "", 400},
		{"null decision", "POST", strings.Replace(payload, `true`, `null`, 1), "valid", "application/json", "", "", 400},
		{"unknown field", "POST", strings.TrimSuffix(payload, "}") + `,"owner":true}`, "valid", "application/json", "", "", 400},
		{"trailing json", "POST", payload + `{}`, "valid", "application/json", "", "", 400},
		{"oversized", "POST", strings.Replace(payload, "private-subject", strings.Repeat("s", 4096), 1), "valid", "application/json", "", "", 400},
		{"unknown network", "POST", strings.Replace(payload, network.ID, "missing", 1), "valid", "application/json", "", "", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := request(tc.method, tc.body, tc.cookie, tc.media, tc.origin, tc.site)
			if rec.Code != tc.want {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if len(st.data.Memberships) != 0 {
				t.Fatal("rejected request mutated membership")
			}
		})
	}
	for _, origin := range []string{"https://control.invalid", ""} {
		rec := request("POST", payload, "valid", "application/json; charset=utf-8", origin, "same-origin")
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("grant: %d %s", rec.Code, rec.Body.String())
		}
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.IdentityDevices(network.ID, "https://principal.invalid", "private-subject"); err != nil {
		t.Fatal("grant did not persist", err)
	}
	snapshot, _ := json.Marshal(st.Snapshot())
	audit, err := os.ReadFile(app.auditPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"https://principal.invalid", "private-subject"} {
		if strings.Contains(string(snapshot), private) || strings.Contains(string(audit), private) {
			t.Fatal("principal leaked to snapshot or audit")
		}
	}
	before, _ := json.Marshal(st.data)
	st.path = filepath.Join(path, "invalid")
	revoke := strings.Replace(payload, `true`, `false`, 1)
	if rec := request("POST", revoke, "valid", "application/json", "", ""); rec.Code != 500 {
		t.Fatalf("failed save: %d", rec.Code)
	}
	after, _ := json.Marshal(st.data)
	if string(before) != string(after) {
		t.Fatal("failed HTTP revoke mutated membership")
	}
	st.path = path
	if rec := request("POST", revoke, "valid", "application/json", "https://control.invalid", "same-origin"); rec.Code != 200 {
		t.Fatalf("revoke: %d", rec.Code)
	}
	reopened, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.IdentityDevices(network.ID, "https://principal.invalid", "private-subject"); err != ErrUnauthorized {
		t.Fatal("revocation did not persist")
	}
}
