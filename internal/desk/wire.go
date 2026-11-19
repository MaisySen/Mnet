package desk

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ErrAuthorityRetired reports input refusal, not a native-release failure.
var ErrAuthorityRetired = errors.New("desk primary input authority retired")

const wireVersion = "mnet-desk-1"
const wireDeadline = 2 * time.Second

// CertificateKey pins the TLS public key, not a discovery name or IP. The TLS
// handshake proves possession. Production certificate enrollment/UI and the
// Multies authority verifier are deliberately not supplied by this package.
func CertificateKey(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	h := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(h[:])
}
func authenticate(conn *tls.Conn, peer string) error {
	if conn == nil || !digest(peer) {
		return errors.New("desk TLS peer absent")
	}
	if err := conn.SetDeadline(time.Now().Add(wireDeadline)); err != nil {
		return err
	}
	if err := conn.Handshake(); err != nil {
		return errors.New("desk TLS handshake failed")
	}
	state := conn.ConnectionState()
	if state.Version < tls.VersionTLS13 || state.NegotiatedProtocol != wireVersion || len(state.PeerCertificates) != 1 || CertificateKey(state.PeerCertificates[0]) != peer {
		return errors.New("desk TLS peer or protocol differs")
	}
	return nil
}

type wireMessage struct {
	Kind    string   `json:"kind"`
	Binding *Binding `json:"binding,omitempty"`
	Input   *Input   `json:"input,omitempty"`
	OK      bool     `json:"ok,omitempty"`
}

func writeWire(w io.Writer, m wireMessage) error {
	data, err := json.Marshal(m)
	if err != nil || len(data) > 4096 {
		return errors.New("desk message exceeds bound")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	// io.Copy handles legal short writes rather than assuming one TLS Write.
	_, err = io.Copy(w, bytes.NewReader(append(length[:], data...)))
	return err
}
func readWire(r io.Reader) (wireMessage, error) {
	var m wireMessage
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return m, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > 4096 {
		return m, errors.New("desk frame exceeds bound")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return m, err
	}
	if err := validateJSON(data); err != nil {
		return m, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&m)
	if err == nil && m.Kind == "" {
		err = errors.New("desk message kind absent")
	}
	return m, err
}

// Approval is exact, local and single use. Construct it only after independently
// accepting this pair/run/layout/keyboard. Neither TLS nor a hello approves it.
// Any reconnect requires a fresh locally approved Run and Approval object.
type Approval struct {
	binding Binding
	layout  *Compiled
	live    func() bool
	create  func(func() bool) (NativeInput, error)
	used    atomic.Bool
}

func NewApproval(binding Binding, layout *Compiled, live func() bool, create func(func() bool) (NativeInput, error)) (*Approval, error) {
	if err := binding.Validate(layout); err != nil {
		return nil, err
	}
	if live == nil || !live() || create == nil {
		return nil, errors.New("desk local approval absent")
	}
	return &Approval{binding: binding, layout: layout, live: live, create: create}, nil
}

// ServePair owns exactly this connection and releases its backend before it
// returns or acknowledges stop. No video/text/file/clipboard stream exists.
func (a *Approval) ServePair(conn *tls.Conn) (result error) {
	if conn == nil {
		return errors.New("desk TLS unavailable")
	}
	defer conn.NetConn().Close()
	if err := authenticate(conn, a.binding.Primary); err != nil {
		return err
	}
	hello, err := readWire(conn)
	if err != nil || hello.Kind != "hello" || hello.Binding == nil || *hello.Binding != a.binding || hello.Input != nil || hello.OK || !a.live() {
		return errors.New("desk exact local approval mismatch")
	}
	if !a.used.CompareAndSwap(false, true) {
		return errors.New("desk approval already consumed")
	}
	var active atomic.Bool
	active.Store(true)
	defer active.Store(false)
	live := func() bool { return active.Load() && a.live() }
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if !live() {
					conn.NetConn().Close()
					return
				}
			}
		}
	}()
	backend, err := a.create(live)
	if err != nil {
		if nativePresent(backend) {
			err = errors.Join(err, backend.Release())
		}
		return err
	}
	var stopping atomic.Bool
	if terminal := nativeTerminal(backend); terminal != nil {
		go func() {
			select {
			case <-done:
				return
			case <-terminal:
				if stopping.Load() {
					return
				}
				active.Store(false)
				conn.NetConn().Close()
			}
		}()
	}
	var display Display
	for _, d := range a.layout.displays {
		if d.ID == a.binding.Display {
			display = d
		}
	}
	recipient, err := NewRecipient(backend, display.PixelWidth, display.PixelHeight, live)
	if err != nil {
		if nativePresent(backend) {
			err = errors.Join(err, backend.Release())
		}
		return err
	}
	defer func() { active.Store(false); result = errors.Join(result, recipient.Close()) }()
	if !live() {
		return errors.New("desk approval retired during setup")
	}
	if err = writeWire(conn, wireMessage{Kind: "ready", OK: true}); err != nil {
		return err
	}
	for {
		if err = conn.SetDeadline(time.Now().Add(wireDeadline)); err != nil {
			return err
		}
		message, err := readWire(conn)
		if err != nil {
			return errors.New("desk channel interrupted")
		}
		if !live() || message.Binding != nil || message.OK {
			return errors.New("desk message outside active run")
		}
		switch message.Kind {
		case "pulse":
			if message.Input != nil {
				return errors.New("invalid desk pulse")
			}
		case "input":
			if message.Input == nil {
				return errors.New("desk input missing")
			}
			if err = recipient.Apply(*message.Input); err != nil {
				return err
			}
		case "stop":
			if message.Input != nil {
				return errors.New("invalid desk stop")
			}
			stopping.Store(true)
			if err = recipient.Close(); err != nil {
				return err
			}
			return writeWire(conn, wireMessage{Kind: "stop", OK: true})
		default:
			return errors.New("unsupported desk message")
		}
		if err = writeWire(conn, wireMessage{Kind: message.Kind, OK: true}); err != nil {
			return err
		}
	}
}

// PairClient is a NativeInput with acknowledged native effects and release.
// A transport error is terminal; no queued input is retried or replayed.
type PairClient struct {
	released   bool
	releaseErr error
	mu         sync.Mutex
	conn       *tls.Conn
	live       func() bool
	closed     bool
	err        error
	done       chan struct{}
}

func ConnectPair(conn *tls.Conn, binding Binding, layout *Compiled, live func() bool) (*PairClient, error) {
	if conn == nil {
		return nil, errors.New("desk TLS unavailable")
	}
	success := false
	defer func() {
		if !success {
			conn.NetConn().Close()
		}
	}()
	if err := binding.Validate(layout); err != nil {
		return nil, err
	}
	if live == nil || !live() {
		return nil, errors.New("desk primary local approval absent")
	}
	if err := authenticate(conn, binding.Recipient); err != nil {
		return nil, err
	}
	if err := writeWire(conn, wireMessage{Kind: "hello", Binding: &binding}); err != nil {
		return nil, err
	}
	reply, err := readWire(conn)
	if err != nil || reply.Kind != "ready" || !reply.OK || reply.Input != nil || reply.Binding != nil || !live() {
		return nil, errors.New("desk recipient not ready")
	}
	client := &PairClient{conn: conn, live: live, done: make(chan struct{})}
	go func() {
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-client.done:
				return
			case <-tick.C:
				client.mu.Lock()
				err := client.callLocked(wireMessage{Kind: "pulse"})
				client.mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
	success = true
	return client, nil
}
func (c *PairClient) retireLocked(err error) error {
	if !c.closed {
		c.closed = true
		c.err = err
		close(c.done)
		c.conn.NetConn().Close()
	}
	return c.err
}
func (c *PairClient) callLocked(message wireMessage) error {
	if c.closed {
		return errors.Join(errors.New("desk channel retired"), c.err)
	}
	if !c.live() {
		c.stopLocked()
		return c.retireLocked(ErrAuthorityRetired)
	}
	if err := c.conn.SetDeadline(time.Now().Add(wireDeadline)); err != nil {
		return c.retireLocked(err)
	}
	if err := writeWire(c.conn, message); err != nil {
		return c.retireLocked(errors.New("desk send failed"))
	}
	reply, err := readWire(c.conn)
	if err != nil || reply.Kind != message.Kind || !reply.OK || reply.Input != nil || reply.Binding != nil {
		return c.retireLocked(errors.New("desk acknowledgement unavailable"))
	}
	if !c.live() {
		c.stopLocked()
		return c.retireLocked(ErrAuthorityRetired)
	}
	return nil
}
func (c *PairClient) Apply(input Input) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callLocked(wireMessage{Kind: "input", Input: &input})
}

// stopLocked is the sole cleanup-only exception to live input authority. It
// cannot send input or restore a run. A bounded stop ACK proves native release
// even when local authority expired just before the caller requested cleanup.
func (c *PairClient) stopLocked() error {
	if c.released {
		return c.releaseErr
	}
	if c.closed {
		return c.err
	}
	c.released = true
	err := c.conn.SetDeadline(time.Now().Add(wireDeadline))
	if err == nil {
		err = writeWire(c.conn, wireMessage{Kind: "stop"})
	}
	if err == nil {
		var reply wireMessage
		reply, err = readWire(c.conn)
		if err == nil && (reply.Kind != "stop" || !reply.OK || reply.Input != nil || reply.Binding != nil) {
			err = errors.New("desk release acknowledgement unavailable")
		}
	}
	c.releaseErr = err
	return err
}
func (c *PairClient) Release() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.released {
		return c.releaseErr
	}
	if c.closed {
		return c.err
	}
	err := c.stopLocked()
	c.retireLocked(err)
	return err
}

func (c *PairClient) Stopped() <-chan struct{} { return c.done }
