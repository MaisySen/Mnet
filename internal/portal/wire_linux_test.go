//go:build linux

package portal

import (
	"bufio"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// This uses a real, isolated D-Bus daemon, not a scripted Bus. The fake
// portal enforces the documented wire signatures and emits each Response
// BEFORE returning its method reply. No desktop or user bus is touched.
type wirePortal struct {
	conn *dbus.Conn
	fd   dbus.UnixFD
}

func (p *wirePortal) respond(token string, results map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test/" + token)
	if err := p.conn.Emit(path, ifaceRequest+".Response", uint32(0), results); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return path, nil
}
func (p *wirePortal) CreateSession(opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.respond("create", map[string]dbus.Variant{"session_handle": dbus.MakeVariant("/org/freedesktop/portal/desktop/session/test/mnet")})
}
func (p *wirePortal) SelectSources(session dbus.ObjectPath, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return p.respond("select", map[string]dbus.Variant{})
}
func (p *wirePortal) Start(session dbus.ObjectPath, parent string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	type stream struct {
		Node  uint32
		Props map[string]dbus.Variant
	}
	type size struct {
		W int32
		H int32
	}
	return p.respond("start", map[string]dbus.Variant{"streams": dbus.MakeVariant([]stream{{42, map[string]dbus.Variant{"size": dbus.MakeVariant(size{1920, 1080})}}})})
}
func (p *wirePortal) OpenPipeWireRemote(session dbus.ObjectPath, opts map[string]dbus.Variant) (dbus.UnixFD, *dbus.Error) {
	return p.fd, nil
}

func TestPortalRealDBusWire(t *testing.T) {
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon not installed")
	}
	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--print-address=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := bufio.NewScanner(stdout)
	if !lines.Scan() {
		t.Fatal("no private bus address")
	}
	addr := lines.Text()
	server, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	service := &wirePortal{conn: server, fd: dbus.UnixFD(r.Fd())}
	if err = server.Export(service, dbus.ObjectPath(rootPath), ifaceScreenCast); err != nil {
		t.Fatal(err)
	}
	reply, err := server.RequestName(dest, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name: %v %v", reply, err)
	}
	clientConn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	bus, err := newDBusBus(clientConn)
	if err != nil {
		clientConn.Close()
		t.Fatal(err)
	}
	defer bus.Close()
	client := NewClient(bus)
	client.ResponseTimeout = time.Second
	s, err := client.OpenMonitor()
	if err != nil {
		t.Fatal(err)
	}
	fd := os.NewFile(uintptr(s.FD), "test-received-fd")
	if fd == nil {
		t.Fatal("no fd")
	}
	defer fd.Close()
	if s.NodeID != 42 || s.Width != 1920 || s.Height != 1080 {
		t.Fatalf("stream: %+v", s)
	}
	// Round-trip a byte through the FD transported by the real bus.
	if _, err = w.Write([]byte{7}); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if _, err = fd.Read(b[:]); err != nil || b[0] != 7 {
		t.Fatalf("fd byte: %v %v", b, err)
	}
}
