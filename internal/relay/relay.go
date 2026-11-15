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
	ws   *websocket.Conn
	send chan []byte
}

// NewHub makes an empty hub.
func NewHub() *Hub {
	return &Hub{conns: map[string]*conn{}, ByNodeID: map[string]string{}}
}

// Online reports whether a node currently holds a relay connection.
func (h *Hub) Online(pubkey string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.conns[pubkey]
	return ok
}

// OnlineMap returns a pubkey -> online snapshot.
func (h *Hub) OnlineMap() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]bool, len(h.conns))
	for k := range h.conns {
		out[k] = true
	}
	return out
}

// Attach registers a websocket for a node pubkey and pumps it until it
// dies. nodeID may be empty.
func (h *Hub) Attach(nodeID, pubkey string, ws *websocket.Conn, onPacket func(src, dst string, payload []byte)) {
	c := &conn{ws: ws, send: make(chan []byte, 64)}
	h.mu.Lock()
	if old := h.conns[pubkey]; old != nil {
		close(old.send)
	}
	h.conns[pubkey] = c
	if nodeID != "" {
		h.ByNodeID[nodeID] = pubkey
	}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.conns[pubkey] == c {
			delete(h.conns, pubkey)
		}
		h.mu.Unlock()
		ws.Close()
	}()

	go func() {
		for msg := range c.send {
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				return
			}
		}
	}()
	go func() {
		t := time.NewTicker(pingEvery)
		defer t.Stop()
		for range t.C {
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
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
		h.Forward(pubkey, dst, payload)
	}
}

// Forward hands a packet from src to dst. False when dst is offline.
func (h *Hub) Forward(src, dst string, payload []byte) bool {
	h.mu.Lock()
	c := h.conns[dst]
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
	case c.send <- frame:
		return true
	default:
		return false // peer too slow, drop like UDP would
	}
}
