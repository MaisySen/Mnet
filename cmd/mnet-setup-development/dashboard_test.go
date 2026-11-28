//go:build mnet_enrollment_development

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/controlclient"
	"fintex.au/mnet/internal/daemon"
)

func TestDashboardNetworkEndMessageDistinguishesExpiryAndFailedChecks(t *testing.T) {
	for _, tt := range []struct {
		peerReason string
		err        error
		want       string
	}{
		{peerReason: "peer-expired", want: "The original approved peer lease ended."},
		{peerReason: "authorization-changed", want: "The current source removed that peer."},
		{err: context.Canceled, want: "This finite network run stopped"},
		{err: context.DeadlineExceeded, want: "reached its time limit"},
		{err: controlclient.ErrRegistrarDenied, want: "refused the current access check"},
		{err: controlclient.ErrRegistrarInvalid, want: "could not confirm current access with the registrar"},
		{err: fmt.Errorf("test-only details: %w", controlclient.ErrRegistrarUnavailable), want: "could not confirm current access with the registrar"},
	} {
		message := developmentNetworkEndMessage(tt.peerReason, tt.err)
		if !strings.Contains(message, tt.want) || strings.Contains(message, "test-only details") {
			t.Errorf("end message = %q, want outcome %q", message, tt.want)
		}
	}
}

func TestDashboardCapabilityStopAndJoinedTruth(t *testing.T) {
	done := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	d, err := startDevelopmentDashboard(nil, func() []daemon.DevelopmentNetworkPeer {
		return []daemon.DevelopmentNetworkPeer{{Name: "actual-current-peer", IPv4: "100.64.0.1"}}
	}, done, func() { calls.Add(1); close(done); <-release })
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: time.Second}
	response, err := c.Get(d.origin + "/state")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("unauthenticated read")
	}
	response, err = c.Get(d.URL())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("bootstrap failed")
	}
	response, _ = c.Get(d.URL())
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("bootstrap replay")
	}
	read := func() dashboardView {
		r, err := c.Get(d.origin + "/state")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var v dashboardView
		if json.NewDecoder(r.Body).Decode(&v) != nil {
			t.Fatal("state decode")
		}
		return v
	}
	if v := read(); v.State != "active" || len(v.Peers) != 1 {
		t.Fatal(v)
	}
	post := func(origin, csrf string) int {
		req, _ := http.NewRequest("POST", d.origin+"/stop", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Mnet-Run", csrf)
		r, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		return r.StatusCode
	}
	if post("http://evil.invalid", d.csrf) != 403 || post(d.origin, "bad") != 403 || post("", d.csrf) != 403 || calls.Load() != 0 {
		t.Fatal("crossorigin or CSRF changed owner")
	}
	if post(d.origin, d.csrf) != 202 {
		t.Fatal("valid Stop failed")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop callback not invoked")
	}
	if v := read(); v.State != "stopping" || len(v.Peers) != 0 {
		t.Fatal("unjoined owner shown ended", v)
	}
	close(release)
	d.Finish("All owners joined")
	if v := read(); v.State != "ended" || len(v.Peers) != 0 {
		t.Fatal(v)
	}
	if post(d.origin, d.csrf) != 202 || calls.Load() != 1 {
		t.Fatal("Stop replay")
	}
	// Cookies are not port-scoped, so run-specific names and values both matter.
	d2, err := startDevelopmentDashboard(nil, func() []daemon.DevelopmentNetworkPeer { return nil }, make(chan struct{}), func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if d.cookieName == d2.cookieName {
		t.Fatal("cookie name reused across ports")
	}
	response, _ = c.Get(d2.origin + "/state")
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("other run cookie authorized second port")
	}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", d.origin+"/state", nil)
	req.Host = "localhost.attacker.invalid"
	response, _ = c.Do(req)
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("host rebinding")
	}
}
func TestDashboardServesNoCapabilityInPage(t *testing.T) {
	d, err := startDevelopmentDashboard(nil, func() []daemon.DevelopmentNetworkPeer { return nil }, make(chan struct{}), func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	r, err := c.Get(d.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	if strings.Contains(string(raw), d.bootstrap) || strings.Contains(string(raw), d.cookie) {
		t.Fatal("bootstrap/auth cookie leaked into HTML")
	}
	if !strings.Contains(r.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("embedding allowed")
	}
}
