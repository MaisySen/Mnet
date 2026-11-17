//go:build linux

package mobile

import (
	"net"
	"testing"
	"time"
)

func TestMeshDialUsesAssignedSource(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, e := ln.Accept()
		if e == nil {
			accepted <- c
		}
	}()
	conn, err := dialMeshTCP("127.0.0.2", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case peer := <-accepted:
		defer peer.Close()
		if peer.RemoteAddr().(*net.TCPAddr).IP.String() != "127.0.0.2" {
			t.Fatal("assigned source not used")
		}
	case <-time.After(time.Second):
		t.Fatal("connection missing")
	}
	for _, bad := range []string{"", "not-ip", "0.0.0.0", "::1"} {
		if c, e := dialMeshTCP(bad, ln.Addr().String(), time.Second); e == nil {
			c.Close()
			t.Fatal("invalid source accepted")
		}
	}
}
