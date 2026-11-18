package main

import (
	"context"
	"net"
	"sync"
)

// sessionHostOwner owns the accept wait and every accepted socket, including
// sockets still in hello and muxes handed to host workers. No peer data chooses
// what another owner closes. Close never waits on a capture callback.
type sessionHostOwner struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	closed      bool
	listener    net.Listener
	connections map[*ownedSessionConn]struct{}
}

func newSessionHostOwner() *sessionHostOwner {
	ctx, cancel := context.WithCancel(context.Background())
	return &sessionHostOwner{ctx: ctx, cancel: cancel, connections: make(map[*ownedSessionConn]struct{})}
}
func (o *sessionHostOwner) attachListener(listener net.Listener) bool {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		listener.Close()
		return false
	}
	o.listener = listener
	o.mu.Unlock()
	return true
}
func (o *sessionHostOwner) adopt(conn net.Conn) (net.Conn, error) {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		conn.Close()
		return nil, net.ErrClosed
	}
	owned := &ownedSessionConn{Conn: conn, owner: o}
	o.connections[owned] = struct{}{}
	o.mu.Unlock()
	return owned, nil
}
func (o *sessionHostOwner) Close() {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	o.cancel()
	listener := o.listener
	o.listener = nil
	connections := make([]*ownedSessionConn, 0, len(o.connections))
	for conn := range o.connections {
		connections = append(connections, conn)
	}
	o.mu.Unlock()
	if listener != nil {
		listener.Close()
	}
	for _, conn := range connections {
		conn.Close()
	}
}

type ownedSessionConn struct {
	net.Conn
	owner *sessionHostOwner
	once  sync.Once
	err   error
}

func (c *ownedSessionConn) Close() error {
	c.once.Do(func() { c.owner.mu.Lock(); delete(c.owner.connections, c); c.owner.mu.Unlock(); c.err = c.Conn.Close() })
	return c.err
}

type ownedSessionListener struct {
	net.Listener
	owner *sessionHostOwner
}

func (l ownedSessionListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return l.owner.adopt(conn)
}
