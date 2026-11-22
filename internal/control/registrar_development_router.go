//go:build mnet_enrollment_development || multies_native_fixture

package control

// Explicit local-development protocol effects only. No default Server route,
// production caller, fixture switch, environment read or secret provisioning.
import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
	"fintex.au/mnet/internal/netmap"
)

const developmentNodePrefix = "/api/mnet-node-experimental-v1/"

// Caller must wrap this inner handler in SAME Store's finite development ingress.
// Router does not own a new allowance and cannot reset the gate on a request.
func (s *Store) registrarDevelopmentProtocol(serviceID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.Method != http.MethodPost || !(path == registrar.WorkflowPrefix+"begin" || path == registrar.WorkflowPrefix+"submit" || path == registrar.HistoryPath || path == developmentNodePrefix+"map" || path == developmentNodePrefix+"endpoints") {
			w.WriteHeader(404)
			return
		}
		// Gate validates canonical single auth/content/TLS/body/deadline envelope.
		raw, e := io.ReadAll(io.LimitReader(r.Body, contract.ProofWireMaxBytes+1))
		if e != nil || len(raw) > contract.ProofWireMaxBytes {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fail := func() { w.WriteHeader(403) }
		switch path {
		case registrar.WorkflowPrefix + "begin":
			input, e := contract.DecodeWorkflowBeginWire(raw, serviceID)
			if e != nil {
				w.WriteHeader(400)
				return
			}
			secret := strings.TrimPrefix(r.Header.Get("Authorization"), registrar.WorkflowScheme)
			challenge, e := s.beginRegistrarEnrollment(r.Context(), input.Request, secret)
			if e != nil {
				fail()
				return
			}
			_ = json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: challenge})
		case registrar.WorkflowPrefix + "submit":
			input, e := contract.DecodeWorkflowSubmitWire(raw)
			if e != nil {
				w.WriteHeader(400)
				return
			}
			secret := strings.TrimPrefix(r.Header.Get("Authorization"), registrar.WorkflowScheme)
			mac, e := base64.RawURLEncoding.Strict().DecodeString(input.ResponseMAC)
			if e != nil || len(mac) != 32 {
				w.WriteHeader(400)
				return
			}
			var response [32]byte
			copy(response[:], mac)
			result, e := s.submitRegistrarEnrollment(r.Context(), input.AttemptID, input.ChallengeID, secret, response)
			if e != nil {
				fail()
				return
			}
			_ = json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: result.History.Receipt, ReceiptDigest: result.History.Digest, ConfirmationMAC: base64.RawURLEncoding.EncodeToString(result.Confirmation[:])})
		case registrar.HistoryPath:
			input, e := contract.DecodeHistoryRequestWire(raw)
			if e != nil {
				w.WriteHeader(400)
				return
			}
			secret := strings.TrimPrefix(r.Header.Get("Authorization"), registrar.HistoryScheme)
			history, e := s.registrarHistoricalReceipt(input.Challenge, secret)
			if e != nil {
				fail()
				return
			}
			_ = json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version: 1, Receipt: history.Receipt, ReceiptDigest: history.Digest})
		default:
			var input struct {
				Version            int                        `json:"version"`
				Receipt            contract.ReceiptV2         `json:"receipt"`
				ReceiptDigest      string                     `json:"receipt_digest"`
				Endpoints          []string                   `json:"endpoints"`
				EndpointCapability *netmap.EndpointCapability `json:"endpoint_capability,omitempty"`
			}
			// Existing exact recursive shape walker, not lenient encoding/json parsing.
			if !developmentNodeRequestExact(raw, &input) {
				w.WriteHeader(400)
				return
			}
			secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Mnet-Node-Credential-V1 ")
			// Reuse strict historical wire decoder pinned to original stored challenge,
			// then existing current credential owner. Caller receipt never supplies scope.
			// RawMessage wrapper remarshal normalizes whitespace/escapes and may expand
			// near-cap input. Pinned authority is strict decoded shape/equality/digest,
			// NOT raw wire-byte identity. A normalization expansion may refuse size.
			var original map[string]json.RawMessage
			json.Unmarshal(raw, &original)
			historyRaw, _ := json.Marshal(map[string]json.RawMessage{"version": original["version"], "receipt": original["receipt"], "receipt_digest": original["receipt_digest"]})
			ref, _, e := s.registrarRelayReference(historyRaw, secret)
			if e != nil {
				fail()
				return
			}
			if path == developmentNodePrefix+"endpoints" {
				if e := s.setRegistrarCredentialEndpointProfile(ref, secret, input.Endpoints, input.EndpointCapability); e != nil {
					fail()
					return
				}
				_ = json.NewEncoder(w).Encode(struct {
					Version int    `json:"version"`
					Digest  string `json:"receipt_digest"`
					Updated bool   `json:"updated"`
				}{1, input.ReceiptDigest, true})
				return
			}
			if input.Endpoints != nil || input.EndpointCapability != nil {
				w.WriteHeader(400)
				return
			}
			m, e := s.registrarCredentialMap(ref, secret)
			if e != nil {
				fail()
				return
			}
			_ = json.NewEncoder(w).Encode(struct {
				Version int         `json:"version"`
				Digest  string      `json:"receipt_digest"`
				Map     *netmap.Map `json:"map"`
			}{1, input.ReceiptDigest, m})
		}
	})
}

func developmentNodeRequestExact(raw []byte, out any) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return false
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, e = d.Token()
		name, ok := token.(string)
		if e != nil || !ok {
			return false
		}
		if _, dup := fields[name]; dup {
			return false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
		fields[name] = value
	}
	if token, e = d.Token(); e != nil || token != json.Delim('}') {
		return false
	}
	if _, e = d.Token(); e != io.EOF {
		return false
	}
	if len(fields) != 4 && len(fields) != 5 {
		return false
	}
	types := map[string]reflect.Type{"version": reflect.TypeFor[int](), "receipt": reflect.TypeFor[contract.ReceiptV2](), "receipt_digest": reflect.TypeFor[string](), "endpoints": reflect.TypeFor[[]string]()}
	if value, ok := fields["endpoint_capability"]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
		types["endpoint_capability"] = reflect.TypeFor[netmap.EndpointCapability]()
	}
	if len(fields) != len(types) {
		return false
	}
	for name, t := range types {
		value, ok := fields[name]
		if !ok {
			return false
		}
		// Preserve nested syntax for the later strict pinned decoder, rather than
		// round-tripping through ReceiptV2 (which could erase duplicate/unknown keys).
		// The wrapper remarshal can compact/escape raw bytes; see handler comment.
		if name == "receipt" {
			continue
		}
		if name == "endpoints" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		item := json.NewDecoder(bytes.NewReader(value))
		item.UseNumber()
		if !registrarConfigShape(item, t, 0) {
			return false
		}
		if _, e := item.Token(); e != io.EOF {
			return false
		}
	}
	if json.Unmarshal(raw, out) != nil {
		return false
	}
	// Purpose-specific version checked independently of JSON syntax.
	var version int
	return json.Unmarshal(fields["version"], &version) == nil && version == 1
}
