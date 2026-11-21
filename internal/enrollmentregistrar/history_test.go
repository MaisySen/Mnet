package enrollmentregistrar

import (
	"context"
	"crypto/x509"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHistoryExactChannelOneShotAndHistoricalExpiry(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	for _, mode := range []string{"success", "redirect", "wrong-receipt", "unknown", "lost", "cache", "oversize", "bad-tls", "close", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var challenge contract.Challenge
			var secret string
			var calls atomic.Int32
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != HistoryPath || r.URL.RawQuery != "" || r.Header.Get("Authorization") != HistoryScheme+secret || r.ProtoMajor != 1 {
					t.Error("wrong exact history channel")
				}
				for _, header := range []string{"Cookie", "Origin", "Referer", "X-Node-ID", "X-Node-Token"} {
					if r.Header.Get(header) != "" {
						t.Error("ambient authority", header)
					}
				}
				raw, _ := io.ReadAll(r.Body)
				input, err := contract.DecodeHistoryRequestWire(raw)
				if err != nil || input.Challenge != challenge {
					t.Error("not full exact original challenge", err)
				}
				headers(w)
				switch mode {
				case "redirect":
					w.Header().Set("Location", target.URL)
					w.WriteHeader(307)
					return
				case "unknown":
					w.WriteHeader(404)
					return
				case "cache":
					w.Header().Set("Cache-Control", "public")
				case "lost":
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				case "oversize":
					w.Write(make([]byte, contract.ProofWireMaxBytes+1))
					return
				}
				receipt, err := contract.ReceiptV2ForChallenge(challenge, "historical-node", challenge.Nonce, challenge.Binding.CheckedAtMS+1)
				if err != nil {
					t.Error(err)
				}
				wire := contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)}
				if mode == "wrong-receipt" {
					wire.Receipt.NetworkID = "other"
				}
				json.NewEncoder(w).Encode(wire)
			}))
			defer s.Close()
			_, challenge, secret, _ = workflowFixture(t, s.URL)
			// Deliberately expired challenge/access. Lookup is immutable history only.
			challenge.Binding.CheckedAtMS -= 120000
			challenge.Binding.ReserveDeadlineMS -= 120000
			challenge.Binding.AccessUntilMS -= 120000
			challenge.ExpiresAtMS -= 120000
			c, err := NewHistory(s.URL, challenge, secret, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if c.client.Jar != nil || c.transport.Proxy != nil || c.transport.TLSClientConfig.InsecureSkipVerify || c.transport.ForceAttemptHTTP2 || !c.transport.DisableKeepAlives {
				t.Fatal("unsafe history transport")
			}
			roots := x509.NewCertPool()
			roots.AddCert(s.Certificate())
			if mode != "bad-tls" {
				c.transport.TLSClientConfig.RootCAs = roots
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			if mode == "close" {
				c.Close()
			}
			result, err := c.Receipt(ctx)
			if mode == "success" {
				if err != nil || result.Receipt.NodeID != "historical-node" {
					t.Fatal(err)
				}
			} else if err == nil || result != (contract.HistoryReceiptWire{}) {
				t.Fatal("failed history returned DATA", err)
			}
			expected := int32(1)
			if mode == "bad-tls" || mode == "cancel" || mode == "close" {
				expected = 0
			}
			if calls.Load() != expected {
				t.Fatal("history request count", calls.Load(), expected)
			}
			if mode == "cancel" {
				c.Close()
			}
			if _, err := c.Receipt(context.Background()); err == nil || calls.Load() != expected {
				t.Fatal("history automatic/repeated send")
			}
			if targetCalls.Load() != 0 {
				t.Fatal("history followed redirect")
			}
		})
	}
	c, secret := fixture(t, "https://registrar.test")
	for _, origin := range []string{"http://registrar.test", "https://other.test", "https://registrar.test/"} {
		if client, err := NewHistory(origin, c, secret, time.Second); err == nil {
			client.Close()
			t.Fatal("wrong history origin accepted")
		}
	}
	if client, err := NewHistory(c.Binding.RegistrarOrigin, c, "invalid", time.Second); err == nil {
		client.Close()
		t.Fatal("wrong original secret")
	}
}

func TestHistoryDeliveryCopyReplayAndClientBoundary(t *testing.T) {
	var challenge contract.Challenge
	var secret string
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receipt, _ := contract.ReceiptV2ForChallenge(challenge, "historical-node", challenge.Nonce, challenge.Binding.CheckedAtMS+1)
		headers(w)
		json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt)})
	}))
	defer s.Close()
	_, challenge, secret, _ = workflowFixture(t, s.URL)
	for _, mode := range []string{"zero", "copy", "other-client", "close", "success"} {
		t.Run(mode, func(t *testing.T) {
			c, err := NewHistory(s.URL, challenge, secret, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			roots := x509.NewCertPool()
			roots.AddCert(s.Certificate())
			c.transport.TLSClientConfig.RootCAs = roots
			d, err := c.ReceiptOwned(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "zero":
				if _, err := c.ConsumeDelivery(&HistoryDelivery{}); err == nil {
					t.Fatal("zero delivery accepted")
				}
			case "copy":
				copy := *d
				if _, err := c.ConsumeDelivery(&copy); err == nil {
					t.Fatal("copied token accepted")
				}
			case "other-client":
				other, _ := NewHistory(s.URL, challenge, secret, time.Second)
				defer other.Close()
				if _, err := other.ConsumeDelivery(d); err == nil {
					t.Fatal("other client accepted")
				}
			case "close":
				c.Close()
				if _, err := c.ConsumeDelivery(d); err == nil {
					t.Fatal("closed client accepted")
				}
				return
			}
			alias := *c
			got, err := alias.ConsumeDelivery(d)
			if err != nil || got.Receipt.NodeID != "historical-node" {
				t.Fatal("origin client delivery", err)
			}
			if _, err := c.ConsumeDelivery(d); err == nil {
				t.Fatal("shared client replayed delivery")
			}
			if _, err := c.ReceiptOwned(context.Background()); err == nil {
				t.Fatal("owned method retried read")
			}
		})
	}
}
