package mobile

import (
	"encoding/json"
	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
	"net"
	"testing"
	"time"
)

type controlEvents struct{ events chan session.ControlStatus }

func (c *controlEvents) OnControl(raw string) {
	var st session.ControlStatus
	_ = json.Unmarshal([]byte(raw), &st)
	select {
	case c.events <- st:
	default:
	}
}
func waitMobileControl(t *testing.T, v *SessionViewer, state string) session.ControlStatus {
	t.Helper()
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		var st session.ControlStatus
		_ = json.Unmarshal([]byte(v.ControlStatus()), &st)
		if st.State == state {
			return st
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("wanted control %s got %s", state, v.ControlStatus())
	return session.ControlStatus{}
}
func TestMobileBoundControlBridgeRequiresLocalAnswer(t *testing.T) {
	owner, viewerNode := &Node{}, &Node{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hostCB := &activeIdentityCallbacks{ownerCallbacks: ownerCallbacks{entered: make(chan struct{}), release: make(chan struct{})}, started: make(chan struct{})}
	close(hostCB.release)
	hostEvents := &controlEvents{events: make(chan session.ControlStatus, 16)}
	if err := owner.startSessionHostWithControlLookup(ln, "host", hostCB, time.Second, func() []daemon.SessionPeer {
		return []daemon.SessionPeer{{Name: "viewer", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}}
	}, hostEvents); err != nil {
		t.Fatal(err)
	}
	defer owner.StopSessionHost()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	vc := &controlEvents{events: make(chan session.ControlStatus, 16)}
	v, err := viewerNode.connectAuthorizedSessionControl(conn, "viewer", &viewerChecks{}, time.Second, func() []daemon.SessionPeer {
		return []daemon.SessionPeer{{Name: "host", IPv4: "127.0.0.1", PublicKey: fixturePeerKey}}
	}, vc)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	waitMobileControl(t, v, "available")
	if v.SendInputJSON(`{"kind":"pointer","action":"click"}`) == nil {
		t.Fatal("view admission enabled mobile input")
	}
	if v.RequestControl() != nil {
		t.Fatal("request failed")
	}
	var request session.ControlStatus
	select {
	case request = <-hostEvents.events:
	case <-time.After(time.Second):
		t.Fatal("mobile host callback absent")
	}
	if owner.DecideSessionControl("other", request.Request, true) == nil {
		t.Fatal("unbound answer accepted")
	}
	if owner.DecideSessionControl(request.Session, request.Request, true) != nil {
		t.Fatal("local answer rejected")
	}
	waitMobileControl(t, v, "allowed")
	if v.SendInputJSON(`{"kind":"pointer","action":"click"}`) != nil {
		t.Fatal("bound input rejected")
	}
	until := time.Now().Add(time.Second)
	for hostCB.inputs.Load() != 1 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if hostCB.inputs.Load() != 1 {
		t.Fatal("granted input did not reach callback")
	}
	if owner.PauseSessionControl(request.Session, request.Request) != nil {
		t.Fatal("pause failed")
	}
	waitMobileControl(t, v, "paused")
	if owner.DecideSessionControl(request.Session, request.Request, true) != nil {
		t.Fatal("Keep failed")
	}
	waitMobileControl(t, v, "allowed")
	if owner.RevokeSessionControl(request.Session, request.Request) != nil {
		t.Fatal("revoke failed")
	}
	waitMobileControl(t, v, "revoked")
	if v.SendInputJSON(`{"kind":"pointer","action":"click"}`) == nil {
		t.Fatal("revoked mobile input accepted")
	}
	select {
	case <-v.mux.Closed():
		t.Fatal("control revoke stopped viewing")
	default:
	}
	v.Close()
	until = time.Now().Add(time.Second)
	for hostCB.ends.Load() == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if owner.DecideSessionControl(request.Session, request.Request, true) == nil {
		t.Fatal("old closed-session answer accepted")
	}
}
