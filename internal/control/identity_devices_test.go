package control

import (
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

func TestIdentityDeviceQueryRequiresExactNetworkIssuerSubject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	one, _ := s.CreateNetwork("one")
	two, _ := s.CreateNetwork("two")
	var want []string
	for i, tc := range []struct {
		network, issuer, subject string
		legacy, expired          bool
	}{
		{"one", "https://issuer.invalid", "subject", false, false},
		{"one", "https://issuer.invalid", "subject", false, false},
		{"two", "https://issuer.invalid", "subject", false, false},
		{"one", "https://other.invalid", "subject", false, false},
		{"one", "https://issuer.invalid", "other", false, false},
		{"one", "", "", true, false},
		{"one", "https://issuer.invalid", "subject", false, true},
	} {
		key := make([]byte, 32)
		key[0] = byte(i + 1)
		joined, err := s.Join(netmap.JoinRequest{Network: tc.network, Name: "device", PubKey: base64.StdEncoding.EncodeToString(key)})
		if err != nil {
			t.Fatal(err)
		}
		if tc.legacy {
			err = s.ApproveNodeAs(joined.NodeID, "same-email@example.invalid")
		} else {
			err = s.ApproveNodeIdentity(joined.NodeID, "same-email@example.invalid", tc.issuer, tc.subject)
		}
		if err != nil {
			t.Fatal(err)
		}
		if tc.expired {
			if err := s.ExpireNode(joined.NodeID); err != nil {
				t.Fatal(err)
			}
		}
		if i < 2 {
			want = append(want, joined.NodeID)
		}
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ApprovedDevicesForIdentity(one.ID, "https://issuer.invalid", "subject")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("query leaked/omitted identities: %+v", got)
	}
	for _, device := range got {
		if device.ID != want[0] && device.ID != want[1] {
			t.Fatal("unrelated device disclosed")
		}
	}
	got[0].Name = "tampered"
	again, _ := s.ApprovedDevicesForIdentity(one.ID, "https://issuer.invalid", "subject")
	if again[0].Name == "tampered" {
		t.Fatal("query aliases store")
	}
	isolated, _ := s.ApprovedDevicesForIdentity(two.ID, "https://issuer.invalid", "subject")
	if len(isolated) != 1 {
		t.Fatal("network scope lost")
	}
	for _, args := range [][3]string{{"", "i", "s"}, {one.ID, "", "s"}, {one.ID, "i", ""}, {"missing", "i", "s"}} {
		if _, err := s.ApprovedDevicesForIdentity(args[0], args[1], args[2]); err == nil {
			t.Fatal("missing scope accepted")
		}
	}
	// Expiry equality is denied, independent of wall-clock KeyExpired rounding.
	s.mu.Lock()
	now := time.Now()
	s.data.Nodes[want[0]].KeyExpiry = &now
	s.mu.Unlock()
	again, _ = s.ApprovedDevicesForIdentity(one.ID, "https://issuer.invalid", "subject")
	if len(again) != 1 {
		t.Fatal("elapsed expiry retained")
	}
}
