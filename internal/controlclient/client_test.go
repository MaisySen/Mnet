package controlclient

import (
	"context"
	"errors"
	"fmt"
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

func TestJoinRejectsIncompleteCredential(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"approved":true}`, `{"node_id":"id","approved":true}`, `{"node_token":"token","approved":false}`, `{"node_id":" ","node_token":"token"}`, `{"node_id":"id","node_token":"\t"}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer srv.Close()
			c := &Client{Base: srv.URL}
			resp, err := c.Join(context.Background(), netmap.JoinRequest{})
			if err == nil || resp != nil {
				t.Fatal("incomplete credential accepted")
			}
			if strings.Contains(err.Error(), "token\"") {
				t.Fatal("response bytes disclosed in diagnostic")
			}
		})
	}
}

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestTransportFailureDoesNotPrintCredentialURL(t *testing.T) {
	cause := errors.New("fixture-token deliberately included by transport")
	c := &Client{Base: "https://control.example", HTTP: &http.Client{Transport: failingTransport{cause}}}
	_, err := c.JoinStatus(context.Background(), "fixture-node", "fixture-token")
	if err == nil {
		t.Fatal("transport failure accepted")
	}
	if strings.Contains(err.Error(), "fixture-token") || strings.Contains(err.Error(), "token=") || strings.Contains(err.Error(), "fixture-node") {
		t.Fatalf("credential appears in printable error: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatal("safe error lost programmatic cause")
	}
	// Cancellation must remain discoverable without printing its URL wrapper.
	c.HTTP.Transport = failingTransport{context.Canceled}
	_, err = c.JoinStatus(context.Background(), "fixture-node", "fixture-token")
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "fixture-token") {
		t.Fatal("cancellation classification or privacy lost")
	}
}

func TestControlResponseBodyLimit(t *testing.T) {
	for _, extra := range []int{0, 1, 1024} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			const valid = `{"node_id":"fixture-id","node_token":"fixture-token","approved":true}`
			body := valid + strings.Repeat(" ", (4<<20)-len(valid)+extra)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer srv.Close()
			c := &Client{Base: srv.URL}
			resp, err := c.Join(context.Background(), netmap.JoinRequest{})
			if extra == 0 && (err != nil || resp == nil) {
				t.Fatalf("limit-sized body refused: %v", err)
			}
			if extra > 0 && (err == nil || resp != nil) {
				t.Fatal("oversized response silently truncated and accepted")
			}
		})
	}
}

type errorBody struct {
	cause  error
	closed bool
}

func (b *errorBody) Read([]byte) (int, error) { return 0, b.cause }
func (b *errorBody) Close() error             { b.closed = true; return nil }

type responseTransport struct{ body io.ReadCloser }

func (tr responseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: tr.body, Header: make(http.Header)}, nil
}

func TestControlResponseReadFailurePrivacy(t *testing.T) {
	cause := errors.New("reader failed at token=fixture-secret")
	body := &errorBody{cause: cause}
	c := &Client{Base: "https://control.example", HTTP: &http.Client{Transport: responseTransport{body: body}}}
	_, err := c.Join(context.Background(), netmap.JoinRequest{})
	if err == nil || !errors.Is(err, cause) || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatal("response reader error privacy or cause matching failed")
	}
	if !body.closed {
		t.Fatal("failed response body not closed")
	}
}
