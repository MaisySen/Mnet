//go:build linux

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

func processFixtureCommand() *exec.Cmd {
	if executable := os.Getenv("MNET_ATTENDED_TEST_EXECUTABLE"); executable != "" {
		return exec.Command(executable, attendedProcessArg)
	}
	c := exec.Command(os.Args[0], "-test.run=^TestAttendedX11ProcessHarness$")
	c.Env = append(os.Environ(), "MNET_PROCESS_FIXTURE_ROLE=injector")
	return c
}
func TestAttendedX11ProcessHarness(t *testing.T) {
	role := os.Getenv("MNET_PROCESS_FIXTURE_ROLE")
	if role == "" {
		t.Skip("owned subprocess fixture only")
	}
	if os.Getenv("MNET_ATTENDED_X11_TEST") != "1" {
		os.Exit(3)
	}
	switch role {
	case "injector":
		err := runAttendedProcess(os.NewFile(3, "commands"), os.NewFile(4, "replies"), attendedProcessLease)
		if err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "coordinator":
		p, err := newAttendedProcessInjector(processFixtureCommand())
		if err != nil {
			t.Fatal(err)
		}
		err = p.InjectAuthorized(session.InputEvent{Kind: "pointer", Action: "down", Button: "left", X: 20, Y: 20}, func() bool { return true })
		state := struct {
			Coordinator, Injector int
			Accepted              bool
		}{os.Getpid(), p.cmd.Process.Pid, err == nil}
		bytes, _ := json.Marshal(state)
		if err = os.WriteFile(filepath.Join(os.Getenv("MNET_ATTENDED_ARTIFACTS"), "coordinator-ready.json"), bytes, 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Second)
		}
	default:
		os.Exit(3)
	}
}
func TestAttendedX11ProcessCloseAndForeignPreheld(t *testing.T) {
	c := requireSafetyDisplay(t)
	root := xproto.Setup(c).DefaultScreen(c).Root
	if err := xtest.Init(c); err != nil {
		t.Fatal(err)
	}
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "foreign"}[foreign], func(t *testing.T) {
			if foreign {
				if err := xtest.FakeInputChecked(c, xproto.ButtonPress, 1, 0, root, 0, 0, 0).Check(); err != nil {
					t.Fatal(err)
				}
			}
			defer xtest.FakeInput(c, xproto.ButtonRelease, 1, 0, root, 0, 0, 0)
			p, err := newAttendedProcessInjector(processFixtureCommand())
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			err = p.InjectAuthorized(session.InputEvent{Kind: "pointer", Action: "down", Button: "left", X: 20, Y: 20}, func() bool { return true })
			if (err == nil) == foreign {
				t.Fatal("unexpected preheld admission", foreign, err)
			}
			closeErr := p.Close()
			if !foreign && closeErr != nil {
				t.Fatal(closeErr)
			}
			state, err := xproto.QueryPointer(c, root).Reply()
			if err != nil || (state.Mask&xproto.ButtonMask1 != 0) != foreign {
				t.Fatal("process cleanup released wrong input", state, err)
			}
			if err = p.InjectAuthorized(session.InputEvent{Kind: "pointer", Action: "move", X: 25, Y: 25}, func() bool { return true }); err == nil {
				t.Fatal("closed process accepted input")
			}
			if foreign {
				xtest.FakeInputChecked(c, xproto.ButtonRelease, 1, 0, root, 0, 0, 0).Check()
			}
		})
	}
}

func testAttendedProcessInjector() (session.Injector, error) {
	if executable := os.Getenv("MNET_ATTENDED_TEST_EXECUTABLE"); executable != "" {
		return newAttendedProcessInjector(exec.Command(executable, attendedProcessArg))
	}
	return newAttendedProcessInjector(processFixtureCommand())
}

func TestAttendedX11ProcessExclusiveInjectorOwner(t *testing.T) {
	c := requireSafetyDisplay(t)
	p, err := newAttendedProcessInjector(processFixtureCommand())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if other, err := newAttendedProcessInjector(processFixtureCommand()); err == nil {
		other.Close()
		t.Fatal("second cooperating input process admitted")
	}
	atom, err := xproto.InternAtom(c, false, uint16(len(attendedInjectorOwnerAtom)), attendedInjectorOwnerAtom).Reply()
	if err != nil {
		t.Fatal(err)
	}
	state, err := xproto.GetSelectionOwner(c, atom.Atom).Reply()
	if err != nil || state.Owner == 0 {
		t.Fatal("surviving child has no own selection", state, err)
	}
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = xproto.GetSelectionOwner(c, atom.Atom).Reply()
	if err != nil || state.Owner != 0 {
		t.Fatal("child close retained ownership", state, err)
	}
	next, err := newAttendedProcessInjector(processFixtureCommand())
	if err != nil {
		t.Fatal("closed child ownership not reusable", err)
	}
	next.Close()
}

func TestAttendedX11ProcessOwnerLossReleasesOnlyItsLedger(t *testing.T) {
	c := requireSafetyDisplay(t)
	root := xproto.Setup(c).DefaultScreen(c).Root
	p, err := newAttendedProcessInjector(processFixtureCommand())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err = p.InjectAuthorized(session.InputEvent{Kind: "pointer", Action: "down", Button: "left", X: 20, Y: 20}, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	atom, err := xproto.InternAtom(c, false, uint16(len(attendedInjectorOwnerAtom)), attendedInjectorOwnerAtom).Reply()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := xproto.NewWindowId(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.CreateWindowChecked(c, 0, foreign, root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0, 0, nil).Check(); err != nil {
		t.Fatal(err)
	}
	defer xproto.DestroyWindow(c, foreign)
	if err = xproto.SetSelectionOwnerChecked(c, foreign, atom.Atom, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.exit:
	case <-time.After(2 * time.Second):
		t.Fatal("child kept input after losing own generation")
	}
	state, err := xproto.QueryPointer(c, root).Reply()
	if err != nil || state.Mask&xproto.ButtonMask1 != 0 {
		t.Fatal("owner loss kept own button pressed", state, err)
	}
	owner, err := xproto.GetSelectionOwner(c, atom.Atom).Reply()
	if err != nil || owner.Owner != foreign {
		t.Fatal("old child erased replacement owner", owner, err)
	}
}
func TestAttendedX11ProcessMalformedCommandRetiresOwnedInput(t *testing.T) {
	c := requireSafetyDisplay(t)
	root := xproto.Setup(c).DefaultScreen(c).Root
	p, err := newAttendedProcessInjector(processFixtureCommand())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err = p.InjectAuthorized(session.InputEvent{Kind: "pointer", Action: "down", Button: "left", X: 20, Y: 20}, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	p.writeMu.Lock()
	_, err = p.writer.Write([]byte("invalid-local-command\n"))
	p.writeMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.exit:
	case <-time.After(2 * time.Second):
		t.Fatal("malformed local command did not retire child")
	}
	state, err := xproto.QueryPointer(c, root).Reply()
	if err != nil || state.Mask&xproto.ButtonMask1 != 0 {
		t.Fatal("malformed-command retirement kept own input", state, err)
	}
}
