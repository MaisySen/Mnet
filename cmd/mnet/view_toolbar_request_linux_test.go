//go:build linux

package main

import "testing"

// A host cancellation acknowledgement may advance only the revision after a
// local decline. Asking for a new narrow grant is not input under the old grant.
// Keep that same-session request, but never carry it to another session.
func TestPointerRequestSurvivesSameSessionStatusAckButNotAnotherSession(t *testing.T) {
	for _, same := range []bool{true, false} {
		v := pointerFixture()
		v.closed = true
		v.pointerGrant.State = "declined"
		v.queuePointer(desktopPointerAction{Kind: "request"})
		after := v.pointerGrant
		after.Revision++
		if !same {
			after.Session = "other"
		}
		_ = v.PointerStatus(after)
		actions := v.PointerActions()
		if same && (len(actions) != 1 || actions[0].Kind != "request") {
			t.Fatal("same-session narrow request lost on late host acknowledgement", actions)
		}
		if !same && len(actions) != 0 {
			t.Fatal("request moved to another session", actions)
		}
	}
}
