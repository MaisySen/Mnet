// Package relay moves WireGuard packets between nodes that cannot reach
// each other directly, the same role DERP plays for Tailscale. Packets are
// already WireGuard encrypted, so the relay never sees plaintext.
//
// Wire protocol over one WebSocket:
//
//	client -> server: [32 byte destination pubkey][wireguard packet]
//	server -> client: [32 byte source pubkey][wireguard packet]
package relay

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// KeyLen is the raw size of a WireGuard public key.
	KeyLen = 32
	// MaxPacket bounds a single relayed datagram.
	MaxPacket = 2048

	writeWait = 10 * time.Second
	pongWait  = 60 * time.Second
	pingEvery = 45 * time.Second
)

// EncodeKey turns a raw 32 byte key into its standard base64 form.
func EncodeKey(raw []byte) string { return base64.StdEncoding.EncodeToString(raw) }

// DecodeKey parses a base64 WireGuard key.
func DecodeKey(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != KeyLen {
		return nil, errors.New("bad wireguard key")
	}
	return b, nil
}

// Hub tracks the live relay connections of every node.
type Hub struct {
	mu    sync.Mutex
	conns map[string]*conn // node pubkey (base64) -> connection
	// ByNodeID lets the control server mark presence and clean up on delete.
	ByNodeID map[string]string // node ID -> pubkey
}

type conn struct {
	ws     *websocket.Conn
	send   chan []byte
	done   chan struct{}
	once   sync.Once
	nodeID string
}

func (c *conn) close() { c.once.Do(func() { close(c.done); c.ws.Close() }) }

// NewHub makes an empty hub.
func NewHub() *Hub {
	return &Hub{conns: map[string]*conn{}, ByNodeID: map[string]string{}}
}

// Online reports whether a node currently holds a relay connection.
func (h *Hub) Online(pubkey string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.conns["\x00"+pubkey]
	return ok
}

// OnlineMap returns a pubkey -> online snapshot.
func (h *Hub) OnlineMap() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]bool, len(h.conns))
	for k := range h.conns {
		if i := strings.IndexByte(k, 0); i >= 0 {
			k = k[i+1:]
		}
		out[k] = true
	}
	return out
}

// Attach registers a websocket for a node pubkey and pumps it until it
// dies. nodeID may be empty.
func (h *Hub) Attach(nodeID, pubkey string, ws *websocket.Conn, onPacket func(src, dst string, payload []byte)) {
	h.AttachScoped(nodeID, "", pubkey, ws, onPacket)
}

// AttachScoped isolates identical public keys and destinations by network.
func (h *Hub) AttachScoped(nodeID, scope, pubkey string, ws *websocket.Conn, onPacket func(src, dst string, payload []byte)) {
	key := scope + "\x00" + pubkey
	c := &conn{ws: ws, send: make(chan []byte, 64), done: make(chan struct{}), nodeID: nodeID}
	h.mu.Lock()
	if old := h.conns[key]; old != nil {
		old.close()
		if old.nodeID != "" {
			delete(h.ByNodeID, old.nodeID)
		}
	}
	h.conns[key] = c
	if nodeID != "" {
		h.ByNodeID[nodeID] = key
	}
	h.mu.Unlock()

	var writer sync.WaitGroup
	writer.Add(1)
	defer func() {
		c.close()
		writer.Wait()
		h.mu.Lock()
		if h.conns[key] == c {
			delete(h.conns, key)
			if nodeID != "" {
				delete(h.ByNodeID, nodeID)
			}
		}
		h.mu.Unlock()
	}()
	// Gorilla permits one concurrent data writer. Ping and data share
	// this loop; closing the socket interrupts either a blocked read/write.
	go func() {
		defer writer.Done()
		defer c.close()
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		for {
			var typ int
			var msg []byte
			select {
			case <-c.done:
				return
			case msg = <-c.send:
				typ = websocket.BinaryMessage
			case <-t.C:
				typ = websocket.PingMessage
			}
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(typ, msg); err != nil {
				return
			}
		}
	}()

	ws.SetReadLimit(MaxPacket + KeyLen + 16)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		mt, msg, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.BinaryMessage || len(msg) < KeyLen+1 {
			continue
		}
		dst := EncodeKey(msg[:KeyLen])
		payload := msg[KeyLen:]
		if onPacket != nil {
			onPacket(pubkey, dst, payload)
			continue
		}
		h.ForwardScoped(scope, pubkey, dst, payload)
	}
}

// Detach drops the relay connection for a node, if any. Used when the
// node is deleted so a stale socket cannot keep receiving traffic.
func (h *Hub) Detach(nodeID string) {
	h.mu.Lock()
	pubkey, ok := h.ByNodeID[nodeID]
	var c *conn
	if ok {
		c = h.conns[pubkey]
	}
	h.mu.Unlock()
	if c != nil {
		c.close()
	}
}

// Forward hands a packet from src to dst. False when dst is offline.
func (h *Hub) Forward(src, dst string, payload []byte) bool {
	return h.ForwardScoped("", src, dst, payload)
}

func (h *Hub) ForwardScoped(scope, src, dst string, payload []byte) bool {
	if len(payload) == 0 || len(payload) > MaxPacket {
		return false
	}
	h.mu.Lock()
	c := h.conns[scope+"\x00"+dst]
	h.mu.Unlock()
	if c == nil {
		return false
	}
	srcRaw, err := DecodeKey(src)
	if err != nil {
		return false
	}
	frame := make([]byte, 0, KeyLen+len(payload))
	frame = append(frame, srcRaw...)
	frame = append(frame, payload...)
	select {
	case <-c.done:
		return false
	case c.send <- frame:
		return true
	default:
		return false // peer too slow, drop like UDP would
	}
}
