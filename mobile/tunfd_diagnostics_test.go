//go:build linux

package mobile

import (
	"encoding/json"
	"os"
	"syscall"
	"testing"
)

func TestFDTunPacketDiagnostics(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	tun := newFDTun(fds[0], 1280)
	defer tun.Close()
	peer := os.NewFile(uintptr(fds[1]), "fixture-peer")
	defer peer.Close()
	node := &Node{packetDiag: tun}
	var status map[string]bool
	if json.Unmarshal([]byte(node.PacketDiagnostics()), &status) != nil || status["read_packet"] || status["wrote_packet"] {
		t.Fatal("new tunnel reports traffic")
	}
	if _, err := peer.Write([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	buffers := [][]byte{make([]byte, 20)}
	sizes := make([]int, 1)
	if n, err := tun.Read(buffers, sizes, 4); err != nil || n != 1 || sizes[0] != 3 || buffers[0][4] != 1 {
		t.Fatalf("packet read failed %d %v", n, err)
	}
	if n, err := tun.Write([][]byte{{0, 0, 4, 5}}, 2); err != nil || n != 1 {
		t.Fatal("packet write failed")
	}
	got := make([]byte, 10)
	if n, err := peer.Read(got); err != nil || n != 2 || got[0] != 4 {
		t.Fatal("wrong tunneled bytes")
	}
	if err := json.Unmarshal([]byte(node.PacketDiagnostics()), &status); err != nil || !status["read_packet"] || !status["wrote_packet"] || status["read_failed"] || status["write_failed"] {
		t.Fatal("traffic booleans wrong")
	}
	tun.Close()
	tun.Read(buffers, sizes, 4)
	tun.Write([][]byte{{0, 1}}, 1)
	if err := json.Unmarshal([]byte(node.PacketDiagnostics()), &status); err != nil || !status["read_failed"] || !status["write_failed"] {
		t.Fatal("error booleans wrong")
	}
	if len(status) != 7 {
		t.Fatal("unexpected diagnostics disclosure")
	}
}

func TestNodePacketDiagnosticsWithoutNativeTunnel(t *testing.T) {
	if got := (&Node{}).PacketDiagnostics(); got != "{}" {
		t.Fatal("unexpected native diagnostics")
	}
}

func TestFDTunPacketClasses(t *testing.T) {
	tun := &fdTun{}
	for _, bad := range [][]byte{nil, make([]byte, 19), {0x65}, append([]byte{0x4f}, make([]byte, 19)...)} {
		tun.observePacket(bad)
	}
	if tun.readIPv4.Load() || tun.readTCP.Load() || tun.readSYN.Load() {
		t.Fatal("invalid packet class")
	}
	packet := make([]byte, 40)
	packet[0] = 0x45
	packet[9] = 6
	packet[33] = 2
	tun.observePacket(packet)
	if !tun.readIPv4.Load() || !tun.readTCP.Load() || !tun.readSYN.Load() {
		t.Fatal("valid TCP SYN class absent")
	}
}
