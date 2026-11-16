// Package controlclient is the daemon's client for the Mnet control plane:
// join, map long polling, endpoint reporting and the relay socket.
package controlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

// ErrUnauthorized marks a 401 from the control plane: the server no
// longer knows this node (state wiped, node deleted or token rotated).
// The daemon answers it by re-joining.
var ErrUnauthorized = errors.New("control: unauthorized")

// Client talks to one control server as one node.
type Client struct {
	Base   string // https://control.example.com
	NodeID string
	Token  string
	HTTP   *http.Client
}

func (c *Client) url(path string) string {
	return strings.TrimRight(c.Base, "/") + path
}

func (c *Client) do(req *http.Request, out any) error {
	if c.NodeID != "" {
		req.Header.Set("X-Node-ID", c.NodeID)
		req.Header.Set("X-Node-Token", c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 70 * time.Second}
	}
	// Control replies may not select a new credential destination. Go's
	// default redirect policy copies custom X-Node-* headers, and 307/308
	// can replay the enrollment body (including an auth key). Clone a
	// caller-supplied client rather than mutating shared configuration.
	requestClient := *hc
	requestClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := requestClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e netmap.APIError
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			if resp.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("%w: %s", ErrUnauthorized, e.Error)
			}
			return fmt.Errorf("control: %s", e.Error)
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		return fmt.Errorf("control: http %d", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Join registers this device with a network.
func (c *Client) Join(ctx context.Context, req netmap.JoinRequest) (*netmap.JoinResponse, error) {
	b, _ := json.Marshal(req)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/join"), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	var out netmap.JoinResponse
	if err := c.do(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// JoinStatus polls approval state.
func (c *Client) JoinStatus(ctx context.Context, nodeID, token string) (*netmap.JoinStatus, error) {
	q := url.Values{"node_id": {nodeID}, "token": {token}}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/api/join/status?"+q.Encode()), nil)
	if err != nil {
		return nil, err
	}
	var out netmap.JoinStatus
	if err := c.do(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitMap long polls for the network map newer than since.
func (c *Client) WaitMap(ctx context.Context, since int64) (*netmap.Map, error) {
	q := url.Values{"since": {fmt.Sprint(since)}, "wait": {"1"}}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/api/map?"+q.Encode()), nil)
	if err != nil {
		return nil, err
	}
	var out netmap.Map
	if err := c.do(r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReportEndpoints tells the control server our current candidate endpoints.
func (c *Client) ReportEndpoints(ctx context.Context, eps []string) error {
	b, _ := json.Marshal(netmap.EndpointsUpdate{Endpoints: eps})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/endpoints"), bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	return c.do(r, nil)
}

// Relay is one live relay connection.
type Relay struct {
	ws   *websocket.Conn
	mu   sync.Mutex
	done chan struct{}
}

// DialRelay connects to the relay for this node. onPacket gets the source
// pubkey and the raw WireGuard packet for every inbound datagram.
func (c *Client) DialRelay(ctx context.Context, selfPubkey string, onPacket func(src string, pkt []byte)) (*Relay, error) {
	q := url.Values{"node_id": {c.NodeID}, "token": {c.Token}}
	u := strings.Replace(c.url("/api/relay/ws?"+q.Encode()), "http", "ws", 1)
	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, u, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	r := &Relay{ws: ws, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		ws.SetReadLimit(relay.MaxPacket + relay.KeyLen + 16)
		for {
			mt, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.BinaryMessage || len(msg) < relay.KeyLen+1 {
				continue
			}
			onPacket(relay.EncodeKey(msg[:relay.KeyLen]), msg[relay.KeyLen:])
		}
	}()
	return r, nil
}

// Send ships one packet to dstPubkey. Safe for concurrent use.
func (r *Relay) Send(dstPubkey string, pkt []byte) error {
	raw, err := relay.DecodeKey(dstPubkey)
	if err != nil {
		return err
	}
	frame := make([]byte, 0, relay.KeyLen+len(pkt))
	frame = append(frame, raw...)
	frame = append(frame, pkt...)
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.done:
		return fmt.Errorf("relay closed")
	default:
	}
	r.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return r.ws.WriteMessage(websocket.BinaryMessage, frame)
}

// Done closes when the relay connection dies.
func (r *Relay) Done() <-chan struct{} { return r.done }

// Close ends the relay connection.
func (r *Relay) Close() error { return r.ws.Close() }
