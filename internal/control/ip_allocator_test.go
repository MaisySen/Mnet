package control

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

func TestMeshIPChooserBoundsAndExhaustion(t *testing.T) {
	_, prefix, _ := net.ParseCIDR(netmap.VPNNetwork)
	for _, cursor := range []int{-1, 0, 1, (1 << 22) - 3, (1 << 22) - 2, (1 << 22) - 1, int(^uint(0) >> 1)} {
		off, ip, err := nextMeshIP(cursor, (1<<22)-2, map[string]bool{})
		if err != nil || !prefix.Contains(net.ParseIP(ip)) || ip == "100.64.0.0" || ip == "100.127.255.255" || off < 1 || off > (1<<22)-2 {
			t.Fatalf("cursor %d: %d %s %v", cursor, off, ip, err)
		}
	}
	used := map[string]bool{"100.64.0.1": true, "100.64.0.2": true}
	off, ip, err := nextMeshIP(3, 3, used)
	if err != nil || off != 3 || ip != "100.64.0.3" {
		t.Fatalf("wrap collision skip: %d %s %v", off, ip, err)
	}
	used[ip] = true
	if _, _, err := nextMeshIP(0, 3, used); err == nil {
		t.Fatal("exhausted pool allocated address")
	}
	for _, max := range []int{-1, 0, 1 << 22} {
		if _, _, err := nextMeshIP(0, max, nil); err == nil {
			t.Fatal("invalid pool accepted")
		}
	}
}

func TestMeshAllocatorCursorRecoveryPersistsAndRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	network, err := st.CreateNetwork("shared")
	if err != nil {
		t.Fatal(err)
	}
	join := func(key byte) (*netmap.JoinResponse, error) {
		raw := make([]byte, 32)
		raw[0] = key
		return st.Join(netmap.JoinRequest{Network: "shared", Name: "device", PubKey: base64.StdEncoding.EncodeToString(raw)})
	}
	first, err := join(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.data.Nodes[first.NodeID].IPv4 != "100.64.0.1" {
		t.Fatal("first address changed")
	}
	st.data.NextIP[network.ID] = int(^uint(0) >> 1)
	second, err := join(2)
	if err != nil {
		t.Fatal(err)
	}
	if st.data.Nodes[second.NodeID].IPv4 != "100.64.0.2" {
		t.Fatal("cursor recovery duplicated or escaped address")
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	third, err := join(3)
	if err != nil {
		t.Fatal(err)
	}
	if st.data.Nodes[third.NodeID].IPv4 != "100.64.0.3" {
		t.Fatal("recovered cursor not persisted")
	}
	st.data.NextIP[network.ID] = -100
	before, _ := json.Marshal(st.data)
	version := st.Version()
	st.path = filepath.Join(path, "invalid")
	if _, err := join(4); err == nil {
		t.Fatal("failed persistence accepted")
	}
	after, _ := json.Marshal(st.data)
	if string(before) != string(after) || st.Version() != version {
		t.Fatal("failed wrapped allocation mutated state")
	}
}
