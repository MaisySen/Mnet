package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"fintex.au/mnet/internal/daemon"
)

func TestJSONStatusRoundTrip(t *testing.T) {
	st := &daemon.Status{Status: "waiting-approval", Interface: "utun57", NodeName: "device \"one\"\nline", Server: "https://control.example", Network: "home", IPv4: "100.64.0.1", ApprovalURL: "https://control.example/oidc/start?token=private-fixture", LastError: "diagnostic", Peers: []daemon.PeerStatus{{Name: "peer", IPv4: "100.64.0.2"}}}
	var out bytes.Buffer
	if err := writeJSONStatus(&out, st); err != nil {
		t.Fatal(err)
	}
	var decoded daemon.Status
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal("output was not standalone JSON", err)
	}
	if decoded.Interface != st.Interface || decoded.NodeName != st.NodeName || decoded.Status != st.Status || decoded.ApprovalURL != st.ApprovalURL || len(decoded.Peers) != 1 {
		t.Fatal("status fields changed")
	}
	for _, field := range []string{`"private_key"`, `"node_token"`, `"auth_key"`} {
		if strings.Contains(out.String(), field) {
			t.Fatal("saved secret field exposed")
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("fixture write failure") }
func TestJSONStatusWriteAndFlags(t *testing.T) {
	if err := writeJSONStatus(failingWriter{}, &daemon.Status{}); err == nil {
		t.Fatal("write failure lost")
	}
	for _, args := range [][]string{{"--other"}, {"--json", "extra"}} {
		if _, err := statusJSONRequested(args); err == nil {
			t.Fatal("invalid status flag accepted")
		}
	}
	if yes, err := statusJSONRequested([]string{"--json"}); !yes || err != nil {
		t.Fatal("JSON flag rejected")
	}
	if yes, err := statusJSONRequested(nil); yes || err != nil {
		t.Fatal("default status changed")
	}
}

func TestHumanStatusActualInterface(t *testing.T) {
	for _, test := range []struct {
		name         string
		state        daemon.Status
		want, absent string
	}{
		{"actual", daemon.Status{Status: "up", Interface: "utun57", NodeName: "node", IPv4: "100.64.0.1"}, "interface: utun57\n", "interface: utun\n"},
		{"legacy", daemon.Status{Status: "up", NodeName: "node", IPv4: "100.64.0.1"}, "this node: node (100.64.0.1)\n", "interface:"},
		{"down", daemon.Status{Status: "down", Interface: "utun57"}, "status:  down\n", "interface:"},
		{"denied", daemon.Status{Status: "waiting-approval", Interface: "utun57", ApprovalURL: "https://fixture.invalid/approve"}, "interface: utun57\n", "no peers yet"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			writeHumanStatus(&out, &test.state)
			if !strings.Contains(out.String(), test.want) || strings.Contains(out.String(), test.absent) {
				t.Fatal("human status interface contract mismatch", out.String())
			}
		})
	}
}

func TestHumanPeerStatusMissingAndFutureEvidence(t *testing.T) {
	for _, tc := range []struct {
		state              string
		age                int64
		endpoint           string
		relay, online      bool
		want, status, path string
	}{
		{"unavailable", -1, "", false, false, "unavailable", "unknown", "unknown"},
		{"clock-skew", -1, "127.0.0.1:1234", false, false, "clock skew", "unknown", "direct"},
		{"never", -1, "127.0.0.1:1234", false, false, "never", "offline", "direct"},
		{"recent", 0, "127.0.0.1:1234", false, true, "0s ago", "online", "direct"},
		{"stale", 180, "relay://fixture", true, false, "180s ago", "offline", "relay"},
		{"recent", 42, "", false, true, "42s ago", "online", "unknown"},
		// Older daemons omit handshake_state. Preserve their age/sentinel display.
		{"", -1, "127.0.0.1:1234", false, false, "never", "offline", "direct"},
		{"", 1, "127.0.0.1:1234", false, true, "1s ago", "online", "direct"},
	} {
		t.Run(tc.state+tc.want+tc.path, func(t *testing.T) {
			var out bytes.Buffer
			writeHumanStatus(&out, &daemon.Status{Status: "up", Peers: []daemon.PeerStatus{{Name: "fixture", IPv4: "100.64.0.2", LastHandshake: tc.age, HandshakeState: tc.state, Endpoint: tc.endpoint, Online: tc.online, Relay: tc.relay, Rx: 4, Tx: 8}}})
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			row := strings.Fields(lines[len(lines)-1])
			want := strings.Fields("fixture 100.64.0.2 " + tc.status + " " + tc.path + " " + tc.want + " 4/8")
			if strings.Join(row, "|") != strings.Join(want, "|") {
				t.Fatal("peer status evidence changed", out.String())
			}
		})
	}
}
