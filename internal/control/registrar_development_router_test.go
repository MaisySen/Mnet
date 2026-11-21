//go:build mnet_enrollment_development || multies_native_fixture

package control

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestDevelopmentRouterNoLegacyFallback(t *testing.T) {
	handler := (&Store{}).registrarDevelopmentProtocol("service")
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"} {
		for _, path := range []string{"/api/join", "/account/", "/oidc/start", "/join/approve", "/admin/api/nodes/approve", "/api/map", "/api/endpoints", "/api/relay/ws", "/healthz", "/api/mnet-node-experimental-v1/map?legacy=1", "/api/mnet-node-experimental-v1/%6dap"} {
			req := httptest.NewRequest(method, "https://127.0.0.1"+path, bytes.NewReader([]byte(`{}`)))
			req.TLS = &tls.ConnectionState{}
			req.Header.Set("Authorization", "Bearer forged")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != 404 {
				t.Fatalf("fallback %s %s =%d", method, path, w.Code)
			}
		}
	}
}
func TestDevelopmentNodeStrictRequestShape(t *testing.T) {
	type request struct {
		Version   int                `json:"version"`
		Receipt   contract.ReceiptV2 `json:"receipt"`
		Digest    string             `json:"receipt_digest"`
		Endpoints []string           `json:"endpoints"`
	}
	raw, _ := json.Marshal(request{Version: 1})
	var out request
	if !developmentNodeRequestExact(raw, &out) {
		t.Fatal("client map null shape refused")
	}
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"Version":1`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1.0`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"extra":true`), 1)} {
		if developmentNodeRequestExact(bad, &out) {
			t.Fatal("bad client shape accepted")
		}
	}
}

func TestDevelopmentRouterPinnedReceiptRawDuplicatesAndForgedCredentialRefuse(t *testing.T) {
	s, ref, secret := registrarCredentialFixture(t)
	defer s.Close()
	r := s.registrarNodeLocked(ref.NodeID)
	receipt, e := contract.ReceiptV2ForChallenge(r.Consumption.Challenge, r.NodeID, r.ReceiptID, r.CommittedAtMS)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(struct {
		Version   int                `json:"version"`
		Receipt   contract.ReceiptV2 `json:"receipt"`
		Digest    string             `json:"receipt_digest"`
		Endpoints []string           `json:"endpoints"`
	}{1, receipt, r.ReceiptDigest, nil})
	handler := s.registrarDevelopmentProtocol("fixture-registrar")
	if _, e := s.newRegistrarDevelopmentIngress(registrarIngressLimits{MaxRequests: 32, MaxConcurrent: 1, MaxBodyBytes: 16384, MaxStateBytes: 4 << 20, MaxEntries: 8, Timeout: time.Second}, http.NotFoundHandler()); e != nil {
		t.Fatal(e)
	}
	send := func(body []byte, credential string) int {
		req := httptest.NewRequest("POST", "https://127.0.0.1"+developmentNodePrefix+"map", bytes.NewReader(body))
		req.Header.Set("Authorization", "Mnet-Node-Credential-V1 "+credential)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if send(raw, secret) != 200 {
		t.Fatal("actual pinned receipt refused")
	}
	before, _ := os.ReadFile(s.path)
	node, _ := json.Marshal(receipt.NodeID)
	needle := append([]byte(`"node_id":`), node...)
	duplicate := append(append(append([]byte{}, needle...), ','), needle...)
	badDuplicate := bytes.Replace(raw, needle, duplicate, 1)
	if bytes.Equal(raw, badDuplicate) {
		t.Fatal("mutation missed")
	}
	for _, bad := range [][]byte{badDuplicate, bytes.Replace(raw, []byte(`"node_id":`), []byte(`"Node_id":`), 1), bytes.Replace(raw, []byte(`"receipt":{`), []byte(`"receipt":{"unexpected":1,`), 1)} {
		if send(bad, secret) == 200 {
			t.Fatal("receipt raw malformed bytes laundered into canonical receipt")
		}
	}
	if send(raw, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{91}, 32))) == 200 {
		t.Fatal("forged secret accepted")
	}
	after, _ := os.ReadFile(s.path)
	if !bytes.Equal(before, after) {
		t.Fatal("refusals rewrote state")
	}
}
