//go:build linux

// Package portal's godbus-backed Bus. Kept in its own file so the
// negotiation logic in portal.go stays testable without D-Bus.
package portal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// DialSession connects to the user's session bus, where the portal
// frontend lives.
func DialSession() (Bus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	b, err := newDBusBus(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return b, nil
}

type dbusBus struct {
	conn      *dbus.Conn
	signals   chan *dbus.Signal
	done      chan struct{}
	closeOnce sync.Once
}

func newDBusBus(conn *dbus.Conn) (*dbusBus, error) {
	b := &dbusBus{conn: conn, signals: make(chan *dbus.Signal, 32), done: make(chan struct{})}
	// Register before any portal method can emit a Response. A portal may
	// emit before returning its method reply, so watching afterwards loses it.
	conn.Signal(b.signals)
	err := conn.AddMatchSignal(dbus.WithMatchSender(dest), dbus.WithMatchInterface(ifaceRequest), dbus.WithMatchMember("Response"))
	if err != nil {
		conn.RemoveSignal(b.signals)
		return nil, err
	}
	return b, nil
}

func (b *dbusBus) Close() error {
	var err error
	b.closeOnce.Do(func() { close(b.done); b.conn.RemoveSignal(b.signals); err = b.conn.Close() })
	return err
}

func (b *dbusBus) CallOn(path, iface, method string, args ...any) ([]any, error) {
	obj := b.conn.Object(dest, dbus.ObjectPath(path))
	// Portals respond promptly; the wait for the USER is in the
	// Response signal, not the method reply.
	if iface == ifaceScreenCast && (method == "SelectSources" || method == "Start" || method == "OpenPipeWireRemote") && len(args) > 0 {
		// Session handles are object paths on the wire, not D-Bus strings.
		args = append([]any(nil), args...)
		if path, ok := args[0].(string); ok {
			args[0] = dbus.ObjectPath(path)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	call := obj.CallWithContext(ctx, iface+"."+method, 0, args...)
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
	out := make(chan Response, 1)
	stopped := make(chan struct{})
	var once sync.Once
	cancel := func() { once.Do(func() { close(stopped) }) }
	go func() {
		defer close(out)
		for {
			select {
			case <-stopped:
				return
			case <-b.done:
				return
			case sig, ok := <-b.signals:
				if !ok {
					return
				}
				if sig == nil || sig.Name != ifaceRequest+".Response" || string(sig.Path) != requestPath || len(sig.Body) != 2 {
					continue
				}
				code, ok := sig.Body[0].(uint32)
				if !ok {
					continue
				}
				results, ok := sig.Body[1].(map[string]dbus.Variant)
				if !ok {
					continue
				}
				out <- Response{Code: code, Results: unwrapMap(results)}
				return
			}
		}
	}()
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
