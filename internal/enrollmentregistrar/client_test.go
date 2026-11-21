package enrollmentregistrar

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func fixture(t *testing.T, origin string) (contract.Challenge, string) {
	t.Helper()
	raw, err := os.ReadFile("../enrollmentcontract/testdata/native-credential-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := contract.DecodeNativeCredentialDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	c := doc.Challenge
	c.Binding.RegistrarOrigin = origin
	now := time.Now()
	c.Binding.CheckedAtMS = now.Add(-time.Second).UnixMilli()
	c.Binding.ReserveDeadlineMS = now.Add(time.Minute).UnixMilli()
	c.Binding.AccessUntilMS = now.Add(50 * time.Second).UnixMilli()
	c.ExpiresAtMS = now.Add(20 * time.Second).UnixMilli()
	return c, doc.Secret
}
func trusted(t *testing.T, s *httptest.Server, c contract.Challenge, secret string) *Client {
	t.Helper()
	client, err := New(s.URL, c.Binding, secret, Limits{Timeout: time.Second * 2, MaxChallengeLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	client.transport.TLSClientConfig.RootCAs = roots
	t.Cleanup(client.Close)
	return client
}
func headers(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
}
func TestBootstrapExactOwnedChallengeAndSubmissionWire(t *testing.T) {
	var c contract.Challenge
	var secret string
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != scheme+secret || r.ProtoMajor != 1 {
			t.Error("wrong bootstrap destination/auth/method/protocol")
		}
		for _, name := range []string{"X-Node-ID", "X-Node-Token", "Cookie", "Origin", "Referer"} {
			if r.Header.Get(name) != "" {
				t.Error("legacy/ambient authority inherited")
			}
		}
		raw, _ := io.ReadAll(r.Body)
		if bytes.Contains(raw, []byte(secret)) {
			t.Error("bootstrap secret in body")
		}
		if r.Header.Get("Cache-Control") != "no-store" {
			t.Error("cacheable request")
		}
		headers(w)
		w.Header().Set("Set-Cookie", "ignored=1; Secure")
		switch r.URL.Path {
		case prefix + "challenge":
			var fields map[string]json.RawMessage
			json.Unmarshal(raw, &fields)
			if len(fields) != 4 {
				t.Error("unexpected challenge body")
			}
			json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
		case prefix + "response":
			response, err := contract.DecodeResponseWire(raw)
			if err != nil || response.ChallengeID != c.ID {
				t.Error("submission not owned ID-only MAC DATA")
			}
			json.NewEncoder(w).Encode(contract.ConfirmationWire{Version: 4, ChallengeID: c.ID, ConfirmationMAC: c.Nonce})
		default:
			t.Error("wrong path")
		}
	}))
	defer server.Close()
	c, secret = fixture(t, server.URL)
	client := trusted(t, server, c, secret)
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, client), secret) {
			t.Fatal("client formatting exposed secret")
		}
	}
	if client.http.Jar != nil || client.transport.Proxy != nil || client.transport.TLSClientConfig.InsecureSkipVerify || !client.transport.DisableKeepAlives || client.transport.ForceAttemptHTTP2 || len(client.transport.TLSNextProto) != 0 {
		t.Fatal("ambient auth or retry/TLS bypass")
	}
	got, err := client.Challenge(context.Background())
	if err != nil || got != c {
		t.Fatal("challenge DATA", err)
	}
	if _, err := client.Challenge(context.Background()); !errors.Is(err, ErrInvalid) {
		t.Fatal("challenge repeated")
	}
	var mac [32]byte
	copy(mac[:], bytes.Repeat([]byte{85}, 32))
	if _, err := client.Submit(context.Background(), c.Nonce, mac); !errors.Is(err, ErrInvalid) {
		t.Fatal("caller selected another challenge")
	}
	confirmation, err := client.Submit(context.Background(), c.ID, mac)
	if err != nil || confirmation.ChallengeID != c.ID {
		t.Fatal("confirmation DATA", err)
	}
	if _, err := client.Submit(context.Background(), c.ID, mac); !errors.Is(err, ErrInvalid) {
		t.Fatal("submission repeated")
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected send count")
	}
}
func TestBootstrapConcurrentCopiesShareOneShotBudgets(t *testing.T) {
	var c contract.Challenge
	var calls atomic.Int64
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		headers(w)
		if r.URL.Path == prefix+"challenge" {
			json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
		} else {
			json.NewEncoder(w).Encode(contract.ConfirmationWire{Version: 4, ChallengeID: c.ID, ConfirmationMAC: c.Nonce})
		}
	}))
	defer s.Close()
	c, secret := fixture(t, s.URL)
	client := trusted(t, s, c, secret)
	copyClient := *client
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := copyClient.Challenge(context.Background()); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || calls.Load() != 1 {
		t.Fatal("challenge budget multiplied")
	}
	successes.Store(0)
	var mac [32]byte
	mac[0] = 1
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := copyClient.Submit(context.Background(), c.ID, mac); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || calls.Load() != 2 {
		t.Fatal("submission budget multiplied")
	}
	copyClient.Close()
	if _, err := client.Submit(context.Background(), c.ID, mac); err == nil {
		t.Fatal("copy Close failed")
	}
}
func TestBootstrapResponseFailuresRemainSpentAndRedacted(t *testing.T) {
	for _, kind := range []string{"status", "redirect", "wrong-binding", "wrong-origin", "wrong-purpose", "body-limit", "cache", "encoding", "tls-untrusted", "lost-response"} {
		t.Run(kind, func(t *testing.T) {
			var c contract.Challenge
			var calls, sinkCalls atomic.Int64
			sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkCalls.Add(1) }))
			defer sink.Close()
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				headers(w)
				switch kind {
				case "status":
					w.WriteHeader(401)
					w.Write([]byte("sensitive upstream body"))
					return
				case "redirect":
					w.Header().Set("Location", sink.URL)
					w.WriteHeader(307)
					return
				case "wrong-binding":
					changed := c
					changed.Binding.PrincipalID += "x"
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: changed})
					return
				case "wrong-origin":
					changed := c
					changed.Binding.RegistrarOrigin = sink.URL
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: changed})
					return
				case "wrong-purpose":
					json.NewEncoder(w).Encode(contract.ConfirmationWire{Version: 4, ChallengeID: c.ID, ConfirmationMAC: c.Nonce})
					return
				case "body-limit":
					w.Write(bytes.Repeat([]byte(" "), contract.ProofWireMaxBytes+1))
					return
				case "cache":
					w.Header().Set("Cache-Control", "public")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "lost-response":
					h := w.(http.Hijacker)
					conn, _, _ := h.Hijack()
					conn.Close()
					return
				}
				json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
			}))
			defer s.Close()
			var secret string
			c, secret = fixture(t, s.URL)
			var client *Client
			if kind == "tls-untrusted" {
				var err error
				client, err = New(s.URL, c.Binding, secret, Limits{time.Second, time.Minute})
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
			} else {
				client = trusted(t, s, c, secret)
			}
			got, err := client.Challenge(context.Background())
			if err != ErrUnavailable || got != (contract.Challenge{}) {
				t.Fatal("failure admitted challenge", kind, err)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), sink.URL) || strings.Contains(err.Error(), "sensitive") {
				t.Fatal("error leaked source content")
			}
			before := calls.Load()
			if _, err := client.Challenge(context.Background()); err != ErrInvalid || calls.Load() != before || sinkCalls.Load() != 0 {
				t.Fatal("failure retried or redirected")
			}
		})
	}
}
func TestBootstrapCloseCancelsOwnedIOAndNoLateData(t *testing.T) {
	var c contract.Challenge
	started := make(chan struct{})
	release := make(chan struct{})
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		headers(w)
		json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
	}))
	defer s.Close()
	defer close(release)
	c, secret := fixture(t, s.URL)
	client := trusted(t, s, c, secret)
	done := make(chan error, 1)
	go func() { _, err := client.Challenge(context.Background()); done <- err }()
	<-started
	client.Close()
	select {
	case err := <-done:
		if err != ErrUnavailable {
			t.Fatal("Close accepted result", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel request")
	}
	if client.secret != "" || client.pinned != nil {
		t.Fatal("Close retained usable client secret/challenge")
	}
}
func TestBootstrapInvalidInputZeroSends(t *testing.T) {
	c, secret := fixture(t, "https://registrar.invalid")
	for _, origin := range []string{"http://registrar.invalid", "https://other.invalid", "https://registrar.invalid/path"} {
		if _, err := New(origin, c.Binding, secret, Limits{time.Second, time.Minute}); err != ErrInvalid {
			t.Fatal("bad origin admitted")
		}
	}
	for _, wrong := range []string{"", secret + "=", base64.RawURLEncoding.EncodeToString(make([]byte, 32)), c.Binding.CredentialDigest} {
		if _, err := New(c.Binding.RegistrarOrigin, c.Binding, wrong, Limits{time.Second, time.Minute}); err != ErrInvalid {
			t.Fatal("bad secret admitted")
		}
	}
}

func TestBootstrapSubmissionFailureNeverRetriesOrReturnsConfirmation(t *testing.T) {
	for _, kind := range []string{"wrong-id", "wrong-purpose", "lost-response", "late-expiry"} {
		t.Run(kind, func(t *testing.T) {
			var c contract.Challenge
			var sends atomic.Int64
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headers(w)
				if r.URL.Path == prefix+"challenge" {
					json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
					return
				}
				sends.Add(1)
				switch kind {
				case "wrong-id":
					json.NewEncoder(w).Encode(contract.ConfirmationWire{Version: 4, ChallengeID: c.Nonce, ConfirmationMAC: c.Nonce})
				case "wrong-purpose":
					json.NewEncoder(w).Encode(contract.ResponseWire{Version: 4, ChallengeID: c.ID, ResponseMAC: c.Nonce})
				case "lost-response":
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
				case "late-expiry":
					time.Sleep(200 * time.Millisecond)
					json.NewEncoder(w).Encode(contract.ConfirmationWire{Version: 4, ChallengeID: c.ID, ConfirmationMAC: c.Nonce})
				}
			}))
			defer s.Close()
			var secret string
			c, secret = fixture(t, s.URL)
			if kind == "late-expiry" {
				c.ExpiresAtMS = time.Now().Add(150 * time.Millisecond).UnixMilli()
			}
			client := trusted(t, s, c, secret)
			if _, err := client.Challenge(context.Background()); err != nil {
				t.Fatal(err)
			}
			var mac [32]byte
			mac[0] = 1
			if got, err := client.Submit(context.Background(), c.ID, mac); err != ErrUnavailable || got != (contract.ConfirmationWire{}) {
				t.Fatal("bad submit response accepted", err)
			}
			if _, err := client.Submit(context.Background(), c.ID, mac); err != ErrInvalid || sends.Load() != 1 {
				t.Fatal("submission unknown retried")
			}
		})
	}
}

func TestBootstrapLifetimeBoundsAndCancellation(t *testing.T) {
	for _, kind := range []string{"expired", "checked-at-bound", "remaining-bound", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			var c contract.Challenge
			var sends atomic.Int64
			started := make(chan struct{})
			release := make(chan struct{})
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				headers(w)
				if kind == "cancelled" {
					io.Copy(io.Discard, r.Body)
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
					}
				}
				json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
			}))
			defer s.Close()
			defer close(release)
			var secret string
			c, secret = fixture(t, s.URL)
			switch kind {
			case "expired":
				c.ExpiresAtMS = time.Now().Add(-time.Millisecond).UnixMilli()
			case "checked-at-bound":
				c.Binding.CheckedAtMS = time.Now().Add(-2 * time.Minute).UnixMilli()
			case "remaining-bound":
				c.ExpiresAtMS = time.Now().Add(40 * time.Second).UnixMilli()
			}
			client := trusted(t, s, c, secret)
			if kind == "remaining-bound" {
				client.limits.MaxChallengeLifetime = 30 * time.Second
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancelled" {
				go func() { <-started; cancel() }()
			}
			if got, err := client.Challenge(ctx); err != ErrUnavailable || got != (contract.Challenge{}) {
				t.Fatal("expired/unbounded/cancelled response returned", err)
			}
			if _, err := client.Challenge(context.Background()); err != ErrInvalid || sends.Load() != 1 {
				t.Fatal("failed admission repeated")
			}
		})
	}
}
