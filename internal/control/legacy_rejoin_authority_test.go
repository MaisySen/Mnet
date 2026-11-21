package control

import (
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
)

// These characterize existing legacy behavior, not desired registrar policy.
// A still-valid reusable auth key remains independent enrollment authority.
func TestLegacyRejoinAfterDeleteOrExpiryAuthorityMatrix(t *testing.T) {
	for _, action := range []string{"delete", "expire"} {
		for _, mode := range []string{"valid-reusable", "consumed-once", "expired-reusable", "removed-reusable", "credential-only"} {
			for _, restart := range []bool{false, true} {
				name := action + "/" + mode
				if restart {
					name += "/restart"
				}
				t.Run(name, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "state.json")
					s, e := Open(path)
					if e != nil {
						t.Fatal(e)
					}
					network, e := s.CreateNetwork("fixture")
					if e != nil {
						t.Fatal(e)
					}
					key, e := s.CreateAuthKey(network.ID, "fixture-only", mode != "consumed-once", time.Hour)
					if e != nil {
						t.Fatal(e)
					}
					pub := make([]byte, 32)
					pub[0] = 9
					req := netmap.JoinRequest{Network: network.ID, Name: "fixture-device", PubKey: base64.StdEncoding.EncodeToString(pub), AuthKey: key.Key}
					first, e := s.Join(req)
					if e != nil || !first.Approved {
						t.Fatal("initial keyed join failed")
					}
					// Provenance is not created by a legacy auth key, either before or after rejoin.
					if len(s.data.ApprovalIdentities) != 0 {
						t.Fatal("legacy key created principal provenance")
					}
					switch mode {
					case "expired-reusable":
						past := time.Now().Add(-time.Minute)
						s.data.AuthKeys[key.ID].ExpiresAt = &past
						if e = s.saveLocked(); e != nil {
							t.Fatal(e)
						}
					case "removed-reusable":
						if e = s.DeleteAuthKey(key.ID); e != nil {
							t.Fatal(e)
						}
					case "credential-only":
						req.AuthKey = ""
					}
					if action == "delete" {
						e = s.DeleteNode(first.NodeID)
					} else {
						e = s.ExpireNode(first.NodeID)
					}
					if e != nil {
						t.Fatal(e)
					}
					if _, e = s.Map(first.NodeID); e == nil {
						t.Fatal("revoked node still received map before rejoin")
					}
					if restart {
						s, e = Open(path)
						if e != nil {
							t.Fatal(e)
						}
					}
					req.NodeID, req.NodeToken = first.NodeID, first.NodeToken
					result, e := s.Join(req)
					if mode == "valid-reusable" {
						if e != nil || !result.Approved {
							t.Fatal("existing reusable-key authority did not reapprove")
						}
						if action == "delete" && result.NodeID == first.NodeID {
							t.Fatal("deleted identity reused")
						}
						if action == "expire" && result.NodeID != first.NodeID {
							t.Fatal("existing identity replaced")
						}
						if _, e = s.Map(result.NodeID); e != nil {
							t.Fatal("approved rejoin has no map")
						}
					} else {
						if mode != "credential-only" {
							if !errors.Is(e, ErrBadAuthKey) || result != nil {
								t.Fatal("spent/expired/removed key accepted")
							}
							// Match daemon fallback: after keyed rejection, retry the same saved
							// credential without the unusable auth key. This may be pending, not approved.
							req.AuthKey = ""
							result, e = s.Join(req)
						}
						if e != nil || result.Approved {
							t.Fatal("saved credential alone restored approval")
						}
						if _, e = s.Map(result.NodeID); !errors.Is(e, ErrUnauthorized) {
							t.Fatal("pending rejoin received map")
						}
					}
					if len(s.data.ApprovalIdentities) != 0 {
						t.Fatal("legacy retry acquired Multies provenance")
					}
					snapshot := s.Snapshot()
					if len(snapshot.Nodes) != 1 {
						t.Fatal("retry produced unexpected node count")
					}
				})
			}
		}
	}
}
