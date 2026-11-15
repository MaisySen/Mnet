// Package portal talks to the XDG Desktop Portal ScreenCast
// interface over D-Bus: the only sanctioned screen-capture path on
// compositors without the wlr-screencopy protocol (GNOME above all).
// The portal hands back a PipeWire stream; turning that stream into
// frames is the pump's job (see pump.go and cmd/mnetd's portal
// capturer).
//
// The D-Bus exchange itself is pure Go (godbus) and sits behind the
// Bus interface so the whole negotiation state machine is unit
// testable without a session bus. Live end-to-end verification needs
// a real GNOME Wayland session, which CI and the dev sandbox do not
// have; that verification is tracked as pending in docs/.
package portal

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	// dest is the well-known name every portal frontend serves.
	dest = "org.freedesktop.portal.Desktop"
	// rootPath is the portal object every interface lives on.
	rootPath = "/org/freedesktop/portal/desktop"

	ifaceScreenCast = "org.freedesktop.portal.ScreenCast"
	ifaceRequest    = "org.freedesktop.portal.Request"

	// sourceMonitor captures whole outputs (portal source type 1).
	sourceMonitor = 1
	// cursorEmbedded burns the pointer into the frames (cursor mode 2).
	cursorEmbedded = 2
)

// Response is one org.freedesktop.portal.Request.Response signal.
type Response struct {
	// Code is 0 on success, 1 when the user dismissed the dialog,
	// 2 on other failure.
	Code uint32
	// Results carries the response payload (session handle, stream
	// list, ...) with variant values already unwrapped to plain Go
	// values.
	Results map[string]any
}

// Bus is the slice of a D-Bus session connection the portal client
// needs. The godbus implementation lives in bus_dbus.go; tests drive
// the client with a scripted fake.
type Bus interface {
	// Call invokes a portal method on the desktop object and
	// returns the reply body.
	Call(iface, method string, args ...any) ([]any, error)
	// CallOn invokes a portal method on a specific object path
	// (used for session handles).
	CallOn(path, iface, method string, args ...any) ([]any, error)
	// WatchResponse resolves with the Response signal for the
	// given request object path. The returned cancel func stops
	// the watch.
	WatchResponse(requestPath string) (<-chan Response, func(), error)
	// Close releases the connection.
	Close() error
}

// Stream is one negotiated screencast stream: a PipeWire node on a
// connected socket fd, plus its negotiated geometry when the
// compositor reported it.
type Stream struct {
	NodeID uint32
	Width  int
	Height int
	FD     int
	// RestoreToken is the compositor's persist token for the next
	// OpenMonitor call; empty when the compositor does not persist.
	RestoreToken string
	session      string
	bus          Bus
}

// Close ends the portal session and releases the bus.
func (s *Stream) Close() error {
	if s.session != "" {
		// org.freedesktop.portal.Session.Close; failure only
		// means the compositor already tore the session down.
		_, _ = s.bus.CallOn(s.session, "org.freedesktop.portal.Session", "Close")
	}
	return s.bus.Close()
}

// Session returns the portal session handle, for diagnostics.
func (s *Stream) Session() string { return s.session }

// Client negotiates screencast streams.
type Client struct {
	bus Bus
	// ResponseTimeout bounds each wait for a portal response. The
	// user may need time to click through the consent dialog, so
	// this is generous.
	ResponseTimeout time.Duration
	// RestoreToken, when non-empty, asks the compositor to restore a
	// previously granted selection without showing the consent
	// dialog. mnetd persists the token the portal returns in its
	// state directory; an unknown or revoked token simply falls back
	// to the dialog.
	RestoreToken string
}

// NewClient wraps a session bus.
func NewClient(bus Bus) *Client {
	return &Client{bus: bus, ResponseTimeout: 5 * time.Minute}
}

var (
	// ErrDismissed means the user closed the consent dialog.
	ErrDismissed = errors.New("portal request dismissed by the user")
	// ErrNoStreams means the compositor granted nothing to read.
	ErrNoStreams = errors.New("portal granted no streams")
)

// OpenMonitor runs the full negotiation for one monitor stream:
// CreateSession, SelectSources, Start, OpenPipeWireRemote. On GNOME
// this surfaces the screen-share consent dialog to the user once.
func (c *Client) OpenMonitor() (*Stream, error) {
	token := newToken()
	reply, err := c.bus.Call(ifaceScreenCast, "CreateSession", map[string]any{
		"handle_token":         token,
		"session_handle_token": token,
	})
	if err != nil {
		return nil, fmt.Errorf("CreateSession: %w", err)
	}
	resp, err := c.await(reply, "CreateSession")
	if err != nil {
		return nil, err
	}
	session, ok := stringResult(resp.Results, "session_handle")
	if !ok || session == "" {
		return nil, fmt.Errorf("CreateSession response without session_handle")
	}

	selOpts := map[string]any{
		"handle_token": token,
		"types":        uint32(sourceMonitor),
		"multiple":     false,
		"cursor_mode":  uint32(cursorEmbedded),
		// persist_mode 2: the grant survives daemon restarts until
		// the user revokes it in the compositor's settings.
		"persist_mode": uint32(2),
	}
	if c.RestoreToken != "" {
		selOpts["restore_token"] = c.RestoreToken
	}
	opened := false
	defer func() {
		if !opened {
			_, _ = c.bus.CallOn(session, "org.freedesktop.portal.Session", "Close")
		}
	}()
	reply, err = c.bus.Call(ifaceScreenCast, "SelectSources", session, selOpts)
	if err != nil {
		return nil, fmt.Errorf("SelectSources: %w", err)
	}
	if _, err := c.await(reply, "SelectSources"); err != nil {
		return nil, err
	}

	reply, err = c.bus.Call(ifaceScreenCast, "Start", session, "", map[string]any{
		"handle_token": token,
	})
	if err != nil {
		return nil, fmt.Errorf("Start: %w", err)
	}
	resp, err = c.await(reply, "Start")
	if err != nil {
		return nil, err
	}
	node, w, h, err := firstStream(resp.Results)
	if err != nil {
		return nil, err
	}
	// The compositor may issue (or rotate) a restore token with the
	// grant; the caller persists it for the next start.
	restoreToken, _ := stringResult(resp.Results, "restore_token")

	reply, err = c.bus.Call(ifaceScreenCast, "OpenPipeWireRemote", session, map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("OpenPipeWireRemote: %w", err)
	}
	fd, ok := fdResult(reply)
	if !ok {
		return nil, fmt.Errorf("OpenPipeWireRemote reply without a file descriptor")
	}
	opened = true
	return &Stream{NodeID: node, Width: w, Height: h, FD: fd, session: session, bus: c.bus, RestoreToken: restoreToken}, nil
}

// await watches the request object a portal call returned and waits
// for its Response signal.
func (c *Client) await(reply []any, what string) (Response, error) {
	if len(reply) == 0 {
		return Response{}, fmt.Errorf("%s reply without a request path", what)
	}
	path, ok := reply[0].(string)
	if !ok || path == "" {
		return Response{}, fmt.Errorf("%s reply: request path has type %T", what, reply[0])
	}
	ch, cancel, err := c.bus.WatchResponse(path)
	if err != nil {
		return Response{}, fmt.Errorf("%s: watch response: %w", what, err)
	}
	defer cancel()
	timeout := c.ResponseTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return Response{}, fmt.Errorf("%s: portal connection closed", what)
		}
		switch r.Code {
		case 0:
			return r, nil
		case 1:
			return Response{}, ErrDismissed
		default:
			return Response{}, fmt.Errorf("%s: portal response code %d", what, r.Code)
		}
	case <-time.After(timeout):
		return Response{}, fmt.Errorf("%s: no portal response within %s", what, timeout)
	}
}

func stringResult(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// fdResult pulls a Unix fd out of a method reply. godbus delivers
// UnixFD as int32 already duplicated into our process.
func fdResult(reply []any) (int, bool) {
	if len(reply) == 0 {
		return 0, false
	}
	switch v := reply[0].(type) {
	case int32:
		return int(v), true
	case int:
		return v, true
	case uint32:
		return int(v), true
	}
	return 0, false
}

// firstStream extracts the first granted stream's node id and, when
// the compositor reports it, geometry. D-Bus shape: a(ua{sv}).
func firstStream(results map[string]any) (node uint32, w, h int, err error) {
	raw, ok := results["streams"]
	if !ok {
		return 0, 0, 0, ErrNoStreams
	}
	streams, ok := raw.([]any)
	if !ok {
		// godbus decodes the struct array as []struct{...}; the
		// fake bus and our own normalizer use []any. Anything
		// else is a decode surprise worth naming.
		return 0, 0, 0, fmt.Errorf("streams result has type %T", raw)
	}
	if len(streams) == 0 {
		return 0, 0, 0, ErrNoStreams
	}
	entry, ok := streams[0].([]any)
	if !ok || len(entry) != 2 {
		return 0, 0, 0, fmt.Errorf("stream entry has shape %T", streams[0])
	}
	id, ok := entry[0].(uint32)
	if !ok {
		return 0, 0, 0, fmt.Errorf("stream node id has type %T", entry[0])
	}
	props, _ := entry[1].(map[string]any)
	w, h = streamSize(props)
	return id, w, h, nil
}

// streamSize reads the "(ii)" size property when present. Zeroes mean
// the compositor did not say; the pump then has to learn the size
// from the negotiated PipeWire format.
func streamSize(props map[string]any) (int, int) {
	raw, ok := props["size"]
	if !ok {
		return 0, 0
	}
	pair, ok := raw.([]any)
	if !ok || len(pair) != 2 {
		return 0, 0
	}
	w, ok1 := asInt(pair[0])
	h, ok2 := asInt(pair[1])
	if !ok1 || !ok2 {
		return 0, 0
	}
	return w, h
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case uint32:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

func newToken() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand never fails on supported platforms; keep a
		// deterministic-looking fallback anyway.
		return "mnet0000"
	}
	return "mnet" + hex.EncodeToString(b[:])
}
