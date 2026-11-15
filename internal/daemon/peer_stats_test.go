package daemon

import (
	"encoding/base64"
	"encoding/hex"
	"fintex.au/mnet/internal/netmap"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Field names match device.IpcGetOperation in the pinned WireGuard module.
// Incorrect legacy names silently reported every live peer as unhealthy.
func TestParseWireGuardPeerStats(t *testing.T) {
	rawKey := strings.Repeat("12", 32)
	parsed := parsePeerStats("public_key=" + rawKey + "\nendpoint=127.0.0.1:1234\nlast_handshake_time_sec=1794770000\nlast_handshake_time_nsec=123\ntx_bytes=789\nrx_bytes=456\npersistent_keepalive_interval=25\n")
	keyBytes, _ := hex.DecodeString(rawKey)
	key := base64.StdEncoding.EncodeToString(keyBytes)
	st := parsed[key]
	if st == nil || st.handshake != 1794770000 || st.tx != 789 || st.rx != 456 || st.keepalive != 25 || st.endpoint != "127.0.0.1:1234" {
		t.Fatalf("stats=%+v", st)
	}
}

func TestStatsReadsActualDeviceIPC(t *testing.T) {
	d := testDaemon(t)
	key := pubKeyOf(t, 11)
	d.applyMap(&netmap.Map{Peers: []netmap.Node{{PubKey: key, IPv4: "100.64.0.2", Endpoints: []string{"127.0.0.1:12345"}}}})
	st := d.stats()[key]
	if st == nil || !strings.HasSuffix(st.endpoint, "]:12345") || st.keepalive != 25 {
		t.Fatalf("device stats=%+v", st)
	}
	// Make the healthy timestamp fixture current for path-health calculations.
	parsed := parsePeerStats("public_key=" + strings.Repeat("12", 32) + "\nlast_handshake_time_sec=" + strconv.FormatInt(time.Now().Unix(), 10) + "\n")
	for _, p := range parsed {
		if p.handshake == 0 || time.Now().Unix()-p.handshake > 2 {
			t.Fatal("healthy handshake lost")
		}
	}
}
