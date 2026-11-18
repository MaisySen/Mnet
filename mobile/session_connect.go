package mobile

import (
	"context"
	"errors"
	"net"
	"sync"
)

// SessionConnect owns exactly one pending connection attempt. Cancel interrupts
// its dial and pending hello socket, never another viewer or the whole node.
// After ConnectWithControl returns successfully, the returned viewer owns the
// connection and must be closed separately. A handle is single-use.
type SessionConnect struct {
	mu                          sync.Mutex
	node                        *Node
	ctx                         context.Context
	cancel                      context.CancelFunc
	conn                        net.Conn
	started, finished, canceled bool
}

// NewSessionConnect registers a pending attempt owned by this node. The caller
// must call ConnectWithControl once or Cancel to release an unused handle.
func (n *Node) NewSessionConnect() (*SessionConnect, error) {
	sessMu.Lock()
	defer sessMu.Unlock()
	if n.sessionClosed {
		return nil, errors.New("node is closed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	attempt := &SessionConnect{node: n, ctx: ctx, cancel: cancel}
	if n.sessionConnects == nil {
		n.sessionConnects = make(map[*SessionConnect]struct{})
	}
	n.sessionConnects[attempt] = struct{}{}
	return attempt, nil
}
func (a *SessionConnect) remove() { sessMu.Lock(); delete(a.node.sessionConnects, a); sessMu.Unlock() }

// Cancel is idempotent. It affects only work not yet transferred to a viewer.
func (a *SessionConnect) Cancel() {
	a.mu.Lock()
	if a.finished {
		a.mu.Unlock()
		return
	}
	a.canceled = true
	conn := a.conn
	a.conn = nil
	cancel := a.cancel
	a.mu.Unlock()
	cancel()
	if conn != nil {
		conn.Close()
	}
	a.remove()
}
func (a *SessionConnect) begin() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.canceled || a.finished || a.started {
		return errors.New("connection attempt ended or already used")
	}
	a.started = true
	return nil
}
func (a *SessionConnect) attach(conn net.Conn) error {
	a.mu.Lock()
	if a.canceled || a.finished {
		a.mu.Unlock()
		conn.Close()
		return errors.New("connection attempt canceled")
	}
	a.conn = conn
	a.mu.Unlock()
	return nil
}
func (a *SessionConnect) complete(v *SessionViewer, err error) (*SessionViewer, error) {
	a.mu.Lock()
	canceled := a.canceled
	a.finished = true
	a.conn = nil
	a.mu.Unlock()
	a.cancel()
	a.remove()
	if canceled {
		if v != nil {
			v.Close()
		}
		return nil, errors.New("connection attempt canceled")
	}
	return v, err
}

// ConnectWithControl blocks for this one attempt. Run it off the UI thread.
// Cancel may be called concurrently, including before this method begins.
func (a *SessionConnect) ConnectWithControl(peerIP string, cb ViewerCallbacks, controlCB ControlCallbacks) (*SessionViewer, error) {
	if err := a.begin(); err != nil {
		return nil, err
	}
	v, err := a.node.connectSessionControlAttempt(peerIP, cb, controlCB, a)
	return a.complete(v, err)
}
