package control

// Private development-only registrar relay handler. No Server registration or
// production caller. Its finite gate must already belong to the SAME Store.
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
	"github.com/gorilla/websocket"
)

var errRegistrarRelayUnavailable = errors.New("private registrar relay temporarily unavailable")

const registrarRelayPath = "/api/mnet-node-relay-experimental-v1/ws"
const registrarRelayScheme = "Mnet-Node-Relay-V1 "
const registrarRelayReferenceHeader = "X-Mnet-Relay-Reference-V1"

func (s *Store) newRegistrarDevelopmentRelayIngress(g *registrarIngress, hub *relay.Hub, maxFrames int) (http.Handler, error) {
	if s == nil || g == nil || g.registrarIngressOwner == nil || g.store != s || hub == nil || maxFrames < 1 || maxFrames > 4096 {
		return nil, errRegistrarEnrollment
	}
	s.mu.Lock()
	valid := s.registrarIngress == g.registrarIngressOwner && s.healthLocked() == nil
	s.mu.Unlock()
	if !valid {
		return nil, errRegistrarEnrollment
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refuse := func(code int) { w.Header().Set("Cache-Control", "no-store"); w.WriteHeader(code) }
		o := g.registrarIngressOwner
		o.mu.Lock()
		if o.closed || o.life.Err() != nil || o.used >= o.limits.MaxRequests || o.active >= o.limits.MaxConcurrent {
			o.mu.Unlock()
			refuse(429)
			return
		}
		o.used++
		o.active++
		o.mu.Unlock()
		defer func() { o.mu.Lock(); o.active--; o.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(r.Context(), o.limits.Timeout)
		defer cancel()
		stop := context.AfterFunc(o.life, cancel)
		defer stop()
		if r.TLS == nil || r.Method != http.MethodGet || r.URL.Path != registrarRelayPath || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.Header.Values("Authorization")) != 1 || len(r.Header.Values(registrarRelayReferenceHeader)) != 1 || len(r.Header.Values("Cookie")) != 0 || len(r.Header.Values("Origin")) != 0 || len(r.Header.Values("Referer")) != 0 || len(r.Header.Values("Content-Encoding")) != 0 {
			refuse(400)
			return
		}
		auth := r.Header.Get("Authorization")
		if len(auth) != len(registrarRelayScheme)+43 || !strings.HasPrefix(auth, registrarRelayScheme) {
			refuse(403)
			return
		}
		secret := strings.TrimPrefix(auth, registrarRelayScheme)
		if !registrarCredentialSecret(secret) {
			refuse(403)
			return
		}
		encoded := r.Header.Get(registrarRelayReferenceHeader)
		if len(encoded) == 0 || len(encoded) > base64.RawURLEncoding.EncodedLen(contract.ProofWireMaxBytes) {
			refuse(400)
			return
		}
		raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
		if err != nil || base64.RawURLEncoding.EncodeToString(raw) != encoded {
			refuse(400)
			return
		}
		ref, node, err := s.registrarRelayReference(raw, secret)
		if err != nil {
			if errors.Is(err, errRegistrarRelayUnavailable) {
				refuse(503)
			} else {
				refuse(403)
			}
			return
		}
		expiry := time.UnixMilli(node.KeyExpiry.UnixMilli())
		ctx, expiryCancel := context.WithDeadline(ctx, expiry)
		defer expiryCancel()
		// Recheck both exact source credential and destination current authority,
		// including ACL, on forwarding and again on queued destination delivery.
		allowed := func(dst string) bool {
			if ctx.Err() != nil {
				return false
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if _, err := s.authorizeRegistrarCredentialLocked(ref, secret); err != nil {
				return false
			}
			return s.relayAllowedLocked(ref.NodeID, dst)
		}
		up := websocket.Upgrader{HandshakeTimeout: o.limits.Timeout, CheckOrigin: func(r *http.Request) bool { return len(r.Header.Values("Origin")) == 0 }}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		// A Store change while the handshake ran must not install a stale socket.
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		_, err = s.authorizeRegistrarCredentialLocked(ref, secret)
		s.mu.Unlock()
		if err != nil {
			return
		}

		_ = hub.AttachScopedBounded(ctx, maxFrames, node.ID, node.NetworkID, node.PubKey, ws, func(src, dst string, payload []byte) {
			if src == node.PubKey && allowed(dst) {
				hub.ForwardScopedChecked(node.NetworkID, src, dst, payload, func() bool { return allowed(dst) })
			}
		})
	}), nil
}

func (s *Store) registrarRelayReference(raw []byte, secret string) (registrarCredentialReference, *netmap.Node, error) {
	var zero registrarCredentialReference
	if len(raw) == 0 || len(raw) > contract.ProofWireMaxBytes {
		return zero, nil, ErrUnauthorized
	}
	// Untrusted NodeID only selects a retained record. Strict full-wire equality
	// against that record's ORIGINAL challenge follows before any authorization.
	var hint contract.HistoryReceiptWire
	if json.Unmarshal(raw, &hint) != nil {
		return zero, nil, ErrUnauthorized
	}
	if !s.mu.TryLock() {
		return zero, nil, errRegistrarRelayUnavailable
	}
	defer s.mu.Unlock()
	if s.registrarIngress == nil || s.healthLocked() != nil || s.registrarIngressBoundLocked(s.registrarIngress.limits) != nil {
		return zero, nil, errRegistrarRelayUnavailable
	}
	record := s.registrarNodeLocked(hint.Receipt.NodeID)
	if record == nil || record.Consumption == nil {
		return zero, nil, ErrUnauthorized
	}
	wire, err := contract.DecodeHistoryReceiptWire(raw, record.Consumption.Challenge)
	if err != nil || wire.ReceiptDigest != record.ReceiptDigest {
		return zero, nil, ErrUnauthorized
	}
	ref := registrarCredentialReference{NodeID: record.NodeID, NetworkID: record.NetworkID, DevicePublicKey: record.DevicePublicKey, Scope: record.Scope, ReceiptID: record.ReceiptID, ReceiptDigest: record.ReceiptDigest, DescriptorDigest: record.DescriptorDigest, TranscriptDigest: record.Consumption.TranscriptDigest}
	node, err := s.authorizeRegistrarCredentialLocked(ref, secret)
	return ref, node, err
}
