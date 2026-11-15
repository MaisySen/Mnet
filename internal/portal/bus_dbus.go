//go:build linux

// Package portal's godbus-backed Bus. Kept in its own file so the
// negotiation logic in portal.go stays testable without D-Bus.
package portal

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

// DialSession connects to the user's session bus, where the portal
// frontend lives.
func DialSession() (Bus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	return &dbusBus{conn: conn}, nil
}

type dbusBus struct{ conn *dbus.Conn }

func (b *dbusBus) Close() error { return b.conn.Close() }

func (b *dbusBus) CallOn(path, iface, method string, args ...any) ([]any, error) {
	obj := b.conn.Object(dest, dbus.ObjectPath(path))
	// Portals respond promptly; the wait for the USER is in the
	// Response signal, not the method reply.
	call := obj.Call(iface+"."+method, 0, args...)
	if call.Err != nil {
		return nil, call.Err
	}
	normalized := normalize(call.Body)
	body, _ := normalized.([]any)
	return body, nil
}

func (b *dbusBus) Call(iface, method string, args ...any) ([]any, error) {
	return b.CallOn(rootPath, iface, method, args...)
}

func (b *dbusBus) WatchResponse(requestPath string) (<-chan Response, func(), error) {
	err := b.conn.AddMatchSignal(
		dbus.WithMatchSender(dest),
		dbus.WithMatchObjectPath(dbus.ObjectPath(requestPath)),
		dbus.WithMatchInterface(ifaceRequest),
		dbus.WithMatchMember("Response"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("add match: %w", err)
	}
	sigs := make(chan *dbus.Signal, 4)
	b.conn.Signal(sigs)
	out := make(chan Response, 1)
	go func() {
		for sig := range sigs {
			if len(sig.Body) != 2 {
				continue
			}
			code, ok := sig.Body[0].(uint32)
			if !ok {
				continue
			}
			results, _ := sig.Body[1].(map[string]dbus.Variant)
			out <- Response{Code: code, Results: unwrapMap(results)}
			return
		}
	}()
	cancel := func() {
		b.conn.RemoveSignal(sigs)
		close(sigs)
		_ = b.conn.RemoveMatchSignal(
			dbus.WithMatchSender(dest),
			dbus.WithMatchObjectPath(dbus.ObjectPath(requestPath)),
			dbus.WithMatchInterface(ifaceRequest),
			dbus.WithMatchMember("Response"),
		)
	}
	return out, cancel, nil
}

// normalize unwraps godbus values into the plain Go shapes the
// client logic (and the test fakes) use: object paths and handles
// become strings, UnixFD becomes int, variants disappear, struct
// arrays become []any rows.
func normalize(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalize(e)
		}
		return out
	case map[string]dbus.Variant:
		return unwrapMap(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalize(e)
		}
		return out
	case dbus.Variant:
		return normalize(t.Value())
	case dbus.ObjectPath:
		return string(t)
	case dbus.UnixFD:
		return int(t)
	default:
		// Structs, struct arrays and typed slices decode into
		// reflected shapes; flatten them into []any rows.
		return normalizeOther(t)
	}
}

func unwrapMap(m map[string]dbus.Variant) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normalize(v.Value())
	}
	return out
}
