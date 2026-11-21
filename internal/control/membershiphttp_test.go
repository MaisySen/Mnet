package control

import (
	"encoding/base64"
	"encoding/json"
	"fintex.au/mnet/internal/netmap"
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
	st, err := openTestStore(t, path)
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
		{"get requires network", "GET", payload, "valid", "application/json", "", "", 400},
		{"form forbidden", "POST", payload, "valid", "text/plain", "", "", 415},
		{"cross origin", "POST", payload, "valid", "application/json", "https://other.invalid", "", 403},
		{"null origin", "POST", payload, "valid", "application/json", "null", "", 403},
		{"cross site", "POST", payload, "valid", "application/json", "", "cross-site", 403},
		{"sibling site", "POST", payload, "valid", "application/json", "", "same-site", 403},
		{"omitted decision", "POST", strings.Replace(payload, `,"allow":true`, "", 1), "valid", "application/json", "", "", 400},
		{"null decision", "POST", strings.Replace(payload, `true`, `null`, 1), "valid", "application/json", "", "", 400},
		{"duplicate decision", "POST", strings.TrimSuffix(payload, "}") + `,"allow":false}`, "valid", "application/json", "", "", 400},
		{"duplicate identical subject", "POST", strings.TrimSuffix(payload, "}") + `,"subject":"private-subject"}`, "valid", "application/json", "", "", 400},
		{"duplicate different network", "POST", strings.TrimSuffix(payload, "}") + `,"network_id":"missing"}`, "valid", "application/json", "", "", 400},
		{"duplicate issuer", "POST", strings.TrimSuffix(payload, "}") + `,"issuer":"https://other.invalid"}`, "valid", "application/json", "", "", 400},
		{"alternate-case subject", "POST", strings.Replace(payload, `"subject"`, `"SUBJECT"`, 1), "valid", "application/json", "", "", 400},
		{"mixed decision case", "POST", strings.TrimSuffix(payload, "}") + `,"ALLOW":false}`, "valid", "application/json", "", "", 400},
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
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.IdentityDevices(network.ID, "https://principal.invalid", "private-subject"); err != nil {
		t.Fatal("grant did not persist", err)
	}
	st = reopened
	app.store = reopened
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
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err = openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.IdentityDevices(network.ID, "https://principal.invalid", "private-subject"); err != ErrUnauthorized {
		t.Fatal("revocation did not persist")
	}
}

func TestAdminMembershipReadScopeAndCopies(t *testing.T) {
	st, err := openTestStore(t, filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, _ := st.CreateNetwork("first")
	second, _ := st.CreateNetwork("second")
	for _, grant := range [][3]string{{first.ID, "https://issuer.example", "b"}, {first.ID, "https://issuer.example", "a"}, {second.ID, "https://issuer.example", "hidden"}} {
		if err := st.SetNetworkMember(grant[0], grant[1], grant[2], true); err != nil {
			t.Fatal(err)
		}
	}
	members, err := st.OperatorMemberships(first.ID)
	if err != nil || len(members) != 2 || members[0].Subject != "a" || members[1].Subject != "b" {
		t.Fatal("operator grants not sorted/scoped")
	}
	members[0].Subject = "mutated"
	next, _ := st.OperatorMemberships(first.ID)
	if next[0].Subject != "a" {
		t.Fatal("operator read aliases private store")
	}
	app, _, _ := NewServer(st, nil, "https://control.invalid", "fixture")
	app.sessions["valid"] = time.Now().Add(time.Hour)
	for _, cookie := range []string{"", "account-session", "device-token", "valid"} {
		req := httptest.NewRequest("GET", "https://control.invalid/admin/api/memberships?network_id="+first.ID, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: cookie})
		}
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if cookie != "valid" {
			if rec.Code != 401 {
				t.Fatal("non-operator read private principals")
			}
			continue
		}
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("operator membership read failed")
		}
		if strings.Contains(rec.Body.String(), "hidden") || strings.Contains(rec.Body.String(), "approval_identities") {
			t.Fatal("operator read crossed requested network/scope")
		}
	}
	if err := st.SetNetworkMember(first.ID, "https://issuer.example", "a", false); err != nil {
		t.Fatal(err)
	}
	members, _ = st.OperatorMemberships(first.ID)
	if len(members) != 1 || members[0].Subject != "b" {
		t.Fatal("revoked grant remained listed")
	}
}

// Real currently valid account and node credentials must not authorize an
// operator read even when deliberately put in the operator cookie slot.
func TestAdminMembershipReadCredentialSeparationAndEmpty(t *testing.T) {
	app, provider, network, _ := accountFixture(t)
	account, err := app.newAccountSession(network, provider, &OIDCClaims{Subject: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.accountDevices(account); err != nil {
		t.Fatal("fixture account invalid", err)
	}
	joined, err := app.store.Join(netmap.JoinRequest{Network: network, Name: "node-credential", PubKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))})
	if err != nil || joined.NodeToken == "" {
		t.Fatal("fixture node credential unavailable", err)
	}
	app.sessions["operator"] = time.Now().Add(time.Hour)
	empty, err := app.store.CreateNetwork("empty")
	if err != nil {
		t.Fatal(err)
	}
	get := func(id, cookieName, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "https://control.example/admin/api/memberships?network_id="+id, nil)
		req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct{ name, token string }{{"mnet_admin", account}, {"mnet_account", account}, {"mnet_admin", joined.NodeToken}} {
		if rec := get(network, tc.name, tc.token); rec.Code != 401 {
			t.Fatalf("non-operator token accepted: %d", rec.Code)
		}
	}
	for _, tc := range []struct {
		id     string
		status int
	}{{"", 400}, {"missing", 404}, {empty.ID, 200}} {
		rec := get(tc.id, "mnet_admin", "operator")
		if rec.Code != tc.status || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("scope response %d", rec.Code)
		}
		if rec.Code == 200 {
			var body struct {
				Memberships []NetworkMember `json:"memberships"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Memberships == nil || len(body.Memberships) != 0 {
				t.Fatal("empty grants not an empty JSON array")
			}
		}
	}
}

func TestAdminMembershipReadUnambiguousQuery(t *testing.T) {
	app, _, network, _ := accountFixture(t)
	app.sessions["operator"] = time.Now().Add(time.Hour)
	for _, query := range []string{"network_id=" + network + "&network_id=" + network, "network_id=" + network + "&network_id=other", "NETWORK_ID=" + network, "network_id=" + network + "&subject=owner", "network_id=" + network + "&bad=%ZZ", "network_id=" + network + ";ignored=1"} {
		req := httptest.NewRequest("GET", "https://control.example/admin/api/memberships?"+query, nil)
		req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: "operator"})
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != 400 || rec.Header().Get("Cache-Control") != "no-store" || strings.Contains(rec.Body.String(), "owner") {
			t.Fatal("ambiguous query returned private grants", rec.Code)
		}
	}
}
