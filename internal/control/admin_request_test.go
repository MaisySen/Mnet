package control

import (
	"encoding/json"
	"fintex.au/mnet/internal/relay"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminWriteOriginAndMethodBoundary(t *testing.T) {
	st, err := Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := NewServer(st, relay.NewHub(), "https://control.invalid", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	app.sessions["valid"] = time.Now().Add(time.Hour)
	paths := []string{"logout", "networks", "authkeys", "authkeys/delete", "nodes/approve", "nodes/deny", "nodes/delete", "nodes/expire", "memberships", "acl", "state", "metrics"}
	before, _ := json.Marshal(st.data)
	for _, path := range paths {
		for _, method := range []string{"GET", "HEAD", "POST", "PUT", "DELETE", "PATCH"} {
			req := httptest.NewRequest(method, "https://control.invalid/admin/api/"+path, strings.NewReader(`{}`))
			req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: "valid"})
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "https://attacker.invalid")
			rec := httptest.NewRecorder()
			app.Handler().ServeHTTP(rec, req)
			expected := 405
			if method == "GET" && (path == "state" || path == "metrics" || path == "acl" || path == "memberships") {
				if rec.Code == 405 || rec.Code == 403 || rec.Code == 401 {
					t.Fatalf("read endpoint blocked: %s %d", path, rec.Code)
				}
				continue
			}
			if (method == "POST" && path != "acl" && path != "state" && path != "metrics") || (method == "PUT" && path == "acl") {
				expected = 403
			}
			if rec.Code != expected {
				t.Fatalf("%s %s: %d want %d", method, path, rec.Code, expected)
			}
		}
	}
	after, _ := json.Marshal(st.data)
	if string(before) != string(after) {
		t.Fatal("rejected admin request changed state")
	}
	// Same-origin and headerless non-browser JSON still work; forms do not.
	for index, tc := range []struct {
		origin, site, media string
		want                int
	}{
		{"https://control.invalid", "same-origin", "application/json", 200},
		{"", "", "application/json", 200},
		{"", "cross-site", "application/json", 403},
		{"null", "", "application/json", 403},
		{"", "", "text/plain", 415},
	} {
		req := httptest.NewRequest("POST", "https://control.invalid/admin/api/networks", strings.NewReader(`{"name":"network-`+fmt.Sprint(index)+`"}`))
		req.AddCookie(&http.Cookie{Name: "mnet_admin", Value: "valid"})
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		req.Header.Set("Content-Type", tc.media)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("write %d want %d: %s", rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestAdminLoginOriginAndSecureCookie(t *testing.T) {
	for _, base := range []string{"https://control.invalid", "http://localhost"} {
		st, err := Open(t.TempDir() + "/state.json")
		if err != nil {
			t.Fatal(err)
		}
		app, _, err := NewServer(st, relay.NewHub(), base, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			origin, media string
			want          int
		}{
			{"https://attacker.invalid", "application/json", 403},
			{"", "text/plain", 415},
			{base, "application/json", 200},
		} {
			req := httptest.NewRequest("POST", base+"/admin/api/login", strings.NewReader(`{"password":"fixture"}`))
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Content-Type", tc.media)
			rec := httptest.NewRecorder()
			app.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("login %d want %d", rec.Code, tc.want)
			}
			if rec.Code == 200 {
				cookies := rec.Result().Cookies()
				if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Secure != strings.HasPrefix(base, "https://") {
					t.Fatal("incorrect admin cookie flags")
				}
			} else if rec.Header().Get("Set-Cookie") != "" || len(app.sessions) != 0 {
				t.Fatal("rejected login created session")
			}
		}
	}
}
