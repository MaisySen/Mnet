package controlclient

// Explicit finite development relay. No daemon startup/retry caller, map relay
// URL support or legacy Client/token path. The native owner must correlate its
// actual State/journal/policy before dial and around packet use.
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/relay"
	"github.com/gorilla/websocket"
)

type RegistrarRelay struct {
	ws        *websocket.Conn
	ctx       context.Context
	client    *RegistrarClient
	mu        sync.Mutex
	frames    atomic.Int32
	maxFrames int32
	done      chan struct{}
	finish    func()
}

func (*RegistrarRelay) String() string          { return "[private finite registrar relay]" }
func (*RegistrarRelay) GoString() string        { return "[private finite registrar relay]" }
func (r *RegistrarRelay) Done() <-chan struct{} { return r.done }
func (r *RegistrarRelay) Close() {
	if r != nil && r.finish != nil {
		r.finish()
	}
}
func (r *RegistrarRelay) Send(dst string, packet []byte) error {
	if r == nil || r.ctx == nil || r.client == nil || r.ws == nil || len(packet) == 0 || len(packet) > relay.MaxPacket {
		return ErrRegistrarInvalid
	}
	key, err := relay.DecodeKey(dst)
	if err != nil || relay.EncodeKey(key) != dst {
		return ErrRegistrarInvalid
	}
	if r.ctx.Err() != nil || r.client.registrarCurrent() != nil || r.frames.Add(1) > r.maxFrames {
		r.Close()
		return ErrRegistrarUnavailable
	}
	frame := make([]byte, 0, relay.KeyLen+len(packet))
	frame = append(frame, key...)
	frame = append(frame, packet...)
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.ctx.Done():
		return ErrRegistrarUnavailable
	case <-r.done:
		return ErrRegistrarUnavailable
	default:
	}
	r.ws.SetWriteDeadline(time.Now().Add(r.client.limits.Timeout))
	if r.ws.WriteMessage(websocket.BinaryMessage, frame) != nil {
		r.Close()
		return ErrRegistrarUnavailable
	}
	return nil
}

func (c *RegistrarClient) DialRegistrarRelay(ctx context.Context, maxFrames int, onPacket func(string, []byte)) (*RegistrarRelay, error) {
	if c == nil || c.registrarClientOwner == nil || ctx == nil || ctx.Err() != nil || maxFrames < 1 || maxFrames > 4096 || onPacket == nil {
		return nil, ErrRegistrarInvalid
	}
	c.mu.Lock()
	if c.denied {
		c.mu.Unlock()
		return nil, ErrRegistrarDenied
	}
	if c.closed || c.lifetime.Err() != nil || time.Now().UnixMilli() < c.document.Receipt.CommittedAtMS || c.used >= c.limits.MaxRequests || c.inflight >= c.limits.MaxConcurrent {
		c.mu.Unlock()
		return nil, ErrRegistrarInvalid
	}
	c.used++
	c.inflight++
	doc, origin, life := c.document, c.origin, c.lifetime
	c.mu.Unlock()
	bounded, cancel := context.WithTimeout(ctx, c.limits.Timeout)
	stopLife := context.AfterFunc(life, cancel)
	var once sync.Once
	var ws *websocket.Conn
	var stopSocket func() bool
	finish := func() {
		once.Do(func() {
			cancel()
			stopLife()
			if stopSocket != nil {
				stopSocket()
			}
			if ws != nil {
				ws.Close()
			}
			c.mu.Lock()
			c.inflight--
			c.mu.Unlock()
		})
	}
	success := false
	defer func() {
		if !success {
			finish()
		}
	}()
	raw, err := json.Marshal(contract.HistoryReceiptWire{Version: 1, Receipt: doc.Receipt, ReceiptDigest: doc.ReceiptDigest})
	if err != nil || len(raw) > contract.ProofWireMaxBytes {
		return nil, ErrRegistrarInvalid
	}
	headers := http.Header{}
	headers.Set("Authorization", "Mnet-Node-Relay-V1 "+doc.Secret)
	headers.Set("X-Mnet-Relay-Reference-V1", base64.RawURLEncoding.EncodeToString(raw))
	headers.Set("Cache-Control", "no-store")
	dialer := websocket.Dialer{Proxy: nil, NetDialContext: c.transport.DialContext, TLSClientConfig: c.transport.TLSClientConfig.Clone(), HandshakeTimeout: c.limits.Timeout, EnableCompression: false}
	var response *http.Response
	ws, response, err = dialer.DialContext(bounded, "wss"+strings.TrimPrefix(origin, "https")+"/api/mnet-node-relay-experimental-v1/ws", headers)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		if response != nil && (response.StatusCode == 401 || response.StatusCode == 403) {
			c.mu.Lock()
			c.closeLocked(true)
			c.mu.Unlock()
			return nil, ErrRegistrarDenied
		}
		return nil, ErrRegistrarUnavailable
	}
	stopSocket = context.AfterFunc(bounded, func() { ws.Close() })
	if bounded.Err() != nil || c.registrarCurrent() != nil {
		return nil, ErrRegistrarUnavailable
	}
	r := &RegistrarRelay{ws: ws, ctx: bounded, client: c, maxFrames: int32(maxFrames), done: make(chan struct{}), finish: finish}
	success = true
	go func() {
		defer close(r.done)
		defer finish()
		ws.SetReadLimit(relay.MaxPacket + relay.KeyLen)
		for {
			typ, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if r.frames.Add(1) > r.maxFrames || c.registrarCurrent() != nil {
				return
			}
			if typ != websocket.BinaryMessage || len(msg) < relay.KeyLen+1 {
				continue
			}
			onPacket(relay.EncodeKey(msg[:relay.KeyLen]), msg[relay.KeyLen:])
		}
	}()
	return r, nil
}
