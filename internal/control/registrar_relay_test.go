package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
	"github.com/gorilla/websocket"
)

func registrarRelayFixture(t *testing.T) (*Store, *httptest.Server, http.Header) {
	t.Helper()
	s, ref, secret := registrarCredentialFixture(t)
	g, err := s.newRegistrarDevelopmentIngress(registrarIngressLimits{MaxRequests: 16, MaxConcurrent: 2, MaxBodyBytes: 16384, MaxStateBytes: 4 << 20, MaxEntries: 8, Timeout: time.Second}, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.newRegistrarDevelopmentRelayIngress(g, relay.NewHub(), 32)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(h)
	t.Cleanup(server.Close)
	r := s.registrarNodeLocked(ref.NodeID)
	receipt, err := contract.ReceiptV2ForChallenge(r.Consumption.Challenge, r.NodeID, r.ReceiptID, r.CommittedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: r.ReceiptDigest})
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set("Authorization", registrarRelayScheme+secret)
	headers.Set(registrarRelayReferenceHeader, base64.RawURLEncoding.EncodeToString(raw))
	return s, server, headers
}

func dialRegistrarRelayFixture(t *testing.T, server *httptest.Server, headers http.Header, paths ...string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	dialer := websocket.Dialer{TLSClientConfig: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone(), HandshakeTimeout: time.Second}
	path := registrarRelayPath
	if len(paths) > 0 {
		path = paths[0]
	}
	return dialer.Dial("wss"+strings.TrimPrefix(server.URL, "https")+path, headers)
}

func TestRegistrarRelayActualCredentialSocketAndClose(t *testing.T) {
	for _, mode := range []string{"Store-close", "expired-connected", "acl-denied-connected"} {
		t.Run(mode, func(t *testing.T) {
			s, server, headers := registrarRelayFixture(t)
			ws, _, err := dialRegistrarRelayFixture(t, server, headers)
			if err != nil {
				t.Fatal(err)
			}
			defer ws.Close()
			var nodeID, key string
			for id, n := range s.data.Nodes {
				nodeID = id
				key = n.PubKey
			}
			rawKey, _ := base64.StdEncoding.DecodeString(key)
			// A real frame round trip proves attachment and current-credential admission.
			packet := append(append([]byte{}, rawKey...), []byte("authenticated-self-frame")...)
			if err := ws.WriteMessage(websocket.BinaryMessage, packet); err != nil {
				t.Fatal(err)
			}
			ws.SetReadDeadline(time.Now().Add(time.Second))
			_, got, err := ws.ReadMessage()
			if err != nil || string(got) != string(packet) {
				t.Fatal("actual credential relay frame", err)
			}
			if mode == "Store-close" {
				s.Close()
			} else if mode == "acl-denied-connected" {
				network := s.data.Nodes[nodeID].NetworkID
				if err := s.SetACL(network, &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "*", Dst: "192.0.2.0/24"}}}); err != nil {
					t.Fatal(err)
				}
				if err := ws.WriteMessage(websocket.BinaryMessage, packet); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := s.ExpireNode(nodeID); err != nil {
					t.Fatal(err)
				}
				if err := ws.WriteMessage(websocket.BinaryMessage, packet); err != nil {
					t.Fatal(err)
				}
			}
			ws.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			if _, _, err := ws.ReadMessage(); err == nil {
				t.Fatal("closed/revoked credential forwarded")
			}
		})
	}
}

func TestRegistrarRelayEnvelopeAndRetainedReferenceRefusals(t *testing.T) {
	for _, mode := range []string{"wrong-purpose", "digest-not-secret", "duplicate-auth", "duplicate-reference", "bad-reference", "altered-receipt", "duplicate-json", "ambient-cookie", "query"} {
		t.Run(mode, func(t *testing.T) {
			_, server, h := registrarRelayFixture(t)
			switch mode {
			case "wrong-purpose":
				h.Set("Authorization", strings.Replace(h.Get("Authorization"), registrarRelayScheme, "Mnet-Node-Credential-V1 ", 1))
			case "digest-not-secret":
				raw, _ := base64.RawURLEncoding.DecodeString(h.Get(registrarRelayReferenceHeader))
				var wire contract.HistoryReceiptWire
				json.Unmarshal(raw, &wire)
				h.Set("Authorization", registrarRelayScheme+wire.ReceiptDigest)
			case "duplicate-auth":
				h.Add("Authorization", h.Get("Authorization"))
			case "duplicate-reference":
				h.Add(registrarRelayReferenceHeader, h.Get(registrarRelayReferenceHeader))
			case "bad-reference":
				h.Set(registrarRelayReferenceHeader, "not-canonical=")
			case "altered-receipt":
				raw, _ := base64.RawURLEncoding.DecodeString(h.Get(registrarRelayReferenceHeader))
				var wire contract.HistoryReceiptWire
				json.Unmarshal(raw, &wire)
				wire.Receipt.ServiceID += "x"
				raw, _ = json.Marshal(wire)
				h.Set(registrarRelayReferenceHeader, base64.RawURLEncoding.EncodeToString(raw))
			case "duplicate-json":
				raw, _ := base64.RawURLEncoding.DecodeString(h.Get(registrarRelayReferenceHeader))
				raw = append([]byte("{\"version\":1,"), raw[1:]...)
				h.Set(registrarRelayReferenceHeader, base64.RawURLEncoding.EncodeToString(raw))
			case "ambient-cookie":
				h.Set("Cookie", "ambient=value")
			}
			path := registrarRelayPath
			if mode == "query" {
				path += "?ignored=1"
			}
			ws, resp, err := dialRegistrarRelayFixture(t, server, h, path)
			if ws != nil {
				ws.Close()
			}
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if err == nil {
				t.Fatal("untrusted relay envelope attached")
			}
		})
	}
}

func TestRegistrarRelayBusyValidDialIsUnavailableNotCredentialDenied(t *testing.T) {
	s, server, h := registrarRelayFixture(t)
	s.mu.Lock()
	done := make(chan int, 1)
	go func() {
		ws, resp, err := dialRegistrarRelayFixture(t, server, h)
		if ws != nil {
			ws.Close()
		}
		if err == nil || resp == nil {
			done <- 0
			return
		}
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	select {
	case status := <-done:
		s.mu.Unlock()
		if status != 503 {
			t.Fatal("valid Store contention misclassified", status)
		}
	case <-time.After(time.Second):
		s.mu.Unlock()
		t.Fatal("busy valid dial queued")
	}
	ws, _, err := dialRegistrarRelayFixture(t, server, h)
	if err != nil {
		t.Fatal("valid credential did not survive transient admission", err)
	}
	ws.Close()
}

func TestRegistrarRelayTwoNodeDestinationDenialMatrix(t *testing.T) {
	for _, mode := range []string{"unapproved", "expired", "deleted", "cross-network", "acl-pair"} {
		t.Run(mode, func(t *testing.T) {
			s, ref, secret := registrarCredentialFixture(t)
			auth, err := s.CreateAuthKey(ref.NetworkID, "destination fixture", false, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			dstKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{71}, 32))
			dst, err := s.Join(netmap.JoinRequest{Network: ref.NetworkID, Name: "destination", PubKey: dstKey, AuthKey: auth.Key})
			if err != nil || !dst.Approved {
				t.Fatal("actual destination join", err)
			}
			hub := relay.NewHub()
			g, err := s.newRegistrarDevelopmentIngress(registrarIngressLimits{MaxRequests: 16, MaxConcurrent: 2, MaxBodyBytes: 16384, MaxStateBytes: 4 << 20, MaxEntries: 8, Timeout: 2 * time.Second}, http.NotFoundHandler())
			if err != nil {
				t.Fatal(err)
			}
			h, err := s.newRegistrarDevelopmentRelayIngress(g, hub, 32)
			if err != nil {
				t.Fatal(err)
			}
			sourceServer := httptest.NewTLSServer(h)
			defer sourceServer.Close()
			legacy, _, err := NewServer(s, hub, "http://localhost", "test-only-admin")
			if err != nil {
				t.Fatal(err)
			}
			destServer := httptest.NewServer(legacy.Handler())
			defer destServer.Close()
			destWS, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(destServer.URL, "http")+"/api/relay/ws?"+url.Values{"node_id": {dst.NodeID}, "token": {dst.NodeToken}}.Encode(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer destWS.Close()
			rawDst, _ := base64.StdEncoding.DecodeString(dstKey)
			destWS.SetReadDeadline(time.Now().Add(time.Second))
			if err := destWS.WriteMessage(websocket.BinaryMessage, append(append([]byte{}, rawDst...), []byte("destination-attach-barrier")...)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := destWS.ReadMessage(); err != nil {
				t.Fatal("destination attachment", err)
			}
			record := s.registrarNodeLocked(ref.NodeID)
			receipt, err := contract.ReceiptV2ForChallenge(record.Consumption.Challenge, record.NodeID, record.ReceiptID, record.CommittedAtMS)
			if err != nil {
				t.Fatal(err)
			}
			wire, _ := json.Marshal(contract.HistoryReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: record.ReceiptDigest})
			headers := http.Header{}
			headers.Set("Authorization", registrarRelayScheme+secret)
			headers.Set(registrarRelayReferenceHeader, base64.RawURLEncoding.EncodeToString(wire))
			sourceWS, _, err := dialRegistrarRelayFixture(t, sourceServer, headers)
			if err != nil {
				t.Fatal(err)
			}
			defer sourceWS.Close()
			send := func(body string) {
				if err := sourceWS.WriteMessage(websocket.BinaryMessage, append(append([]byte{}, rawDst...), []byte(body)...)); err != nil {
					t.Fatal(err)
				}
			}
			send("baseline-allowed")
			destWS.SetReadDeadline(time.Now().Add(time.Second))
			_, first, err := destWS.ReadMessage()
			if err != nil || len(first) < 32 || string(first[32:]) != "baseline-allowed" {
				t.Fatal("two-node baseline", err)
			}
			switch mode {
			case "unapproved":
				s.mu.Lock()
				s.data.Nodes[dst.NodeID].Approved = false
				err = s.saveLocked()
				s.mu.Unlock()
			case "expired":
				err = s.ExpireNode(dst.NodeID)
			case "deleted":
				err = s.DeleteNode(dst.NodeID)
			case "cross-network":
				var other *netmap.Network
				other, err = s.CreateNetwork("other")
				if err == nil {
					s.mu.Lock()
					s.data.Nodes[dst.NodeID].NetworkID = other.ID
					s.relayIndex = nil
					err = s.saveLocked()
					s.mu.Unlock()
				}
			case "acl-pair":
				err = s.SetACL(ref.NetworkID, &netmap.ACLPolicy{Rules: []netmap.ACLRule{{Src: "fixture", Dst: "fixture"}}})
			}
			if err != nil {
				t.Fatal("destination deny transition", err)
			}
			send("new-forbidden-destination-frame")
			destWS.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			if _, packet, err := destWS.ReadMessage(); err == nil {
				t.Fatalf("destination %s received forbidden frame %q", mode, packet[32:])
			}
		})
	}
}
