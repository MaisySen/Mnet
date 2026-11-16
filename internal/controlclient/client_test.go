package controlclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

// The standard client forwards custom X-Node-* headers on redirects. A
// compromised/misconfigured control response must never choose another
// credential destination, even for a same-origin redirect or a custom client.
func TestControlClientRefusesRedirects(t *testing.T) {
	for _, code := range []int{301, 302, 303, 307, 308} {
		for _, custom := range []bool{false, true} {
			for _, sameOrigin := range []bool{false, true} {
				t.Run(http.StatusText(code)+map[bool]string{true: "/custom", false: "/default"}[custom]+map[bool]string{true: "/same-origin", false: "/cross-origin"}[sameOrigin], func(t *testing.T) {
					var leaked, policyCalls atomic.Int32
					destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1); w.Write([]byte(`{}`)) }))
					defer destination.Close()
					var source *httptest.Server
					source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/api/join" {
							leaked.Add(1)
							return
						}
						if r.Header.Get("X-Node-ID") != "fixture-id" || r.Header.Get("X-Node-Token") != "fixture-token" {
							t.Error("initial request missing fixture credentials")
						}
						b, _ := io.ReadAll(r.Body)
						if !strings.Contains(string(b), "fixture-auth") {
							t.Error("initial request missing fixture auth key")
						}
						target := destination.URL + "/collect"
						if sameOrigin {
							target = source.URL + "/collect"
						}
						w.Header().Set("Location", target)
						w.WriteHeader(code)
					}))
					defer source.Close()
					c := &Client{Base: source.URL, NodeID: "fixture-id", Token: "fixture-token"}
					var supplied *http.Client
					if custom {
						supplied = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { policyCalls.Add(1); return nil }}
						c.HTTP = supplied
					}
					if _, err := c.Join(context.Background(), netmap.JoinRequest{AuthKey: "fixture-auth"}); err == nil {
						t.Fatal("redirect accepted")
					}
					if leaked.Load() != 0 {
						t.Fatal("redirect destination was contacted")
					}
					if policyCalls.Load() != 0 {
						t.Fatal("caller redirect policy was allowed to forward credentials")
					}
					if supplied != nil {
						// The caller owns the supplied client. Our request must not replace
						// its redirect policy or modify its other behavior for later uses.
						_ = supplied.CheckRedirect(nil, nil)
						if policyCalls.Load() != 1 {
							t.Fatal("supplied client policy mutated")
						}
					}
				})
			}
		}
	}
}

func TestControlClientDirectRequest(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{true: "custom", false: "default"}[custom], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"node_id":"joined","node_token":"fixture-token","approved":true}`))
			}))
			defer srv.Close()
			c := &Client{Base: srv.URL}
			if custom {
				c.HTTP = srv.Client()
			}
			resp, err := c.Join(context.Background(), netmap.JoinRequest{})
			if err != nil || resp.NodeID != "joined" || !resp.Approved {
				t.Fatalf("direct join failed: %+v %v", resp, err)
			}
		})
	}
}
