package control

import (
	"testing"
	"time"
)

func TestOIDCApprovalRequiresExactLiveClaimedAttempt(t *testing.T) {
	app, p, _, node := accountFixture(t)
	original := oidcState{NodeID: node, Provider: p, Exp: time.Now().Add(time.Minute), claimed: true}
	for _, mode := range []string{"retired", "unclaimed", "expired", "changed-node", "changed-provider", "restored-provider"} {
		t.Run(mode, func(t *testing.T) {
			attempt := original
			app.oidcStates = map[string]oidcState{"state": original}
			switch mode {
			case "retired":
				delete(app.oidcStates, "state")
			case "unclaimed":
				attempt.claimed = false
				app.oidcStates["state"] = attempt
			case "expired":
				attempt.Exp = time.Now()
				app.oidcStates["state"] = attempt
			case "changed-node":
				attempt.NodeID = "other-node"
			case "changed-provider":
				attempt.Provider = &OIDCProvider{}
			case "restored-provider":
				app.SetOIDC(&OIDCProvider{})
				app.SetOIDC(p)
			}
			if err := app.approveOIDCState("state", attempt, &OIDCClaims{Subject: "unexpected-principal"}); err == nil {
				t.Fatal("retired or altered approval accepted")
			}
			identity, exists := app.store.NodeApprovalIdentity(node)
			if !exists || identity.Subject != "owner" {
				t.Fatal("rejected approval changed persisted principal")
			}
		})
	}
}
