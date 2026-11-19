//go:build linux

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func deskFixtureCommand() *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestDeskX11ChildHarness$")
	command.Env = append(os.Environ(), "MNET_DESK_CHILD=1")
	return command
}
func TestDeskX11ChildHarness(t *testing.T) {
	if os.Getenv("MNET_DESK_CHILD") == "" {
		t.Skip("private child fixture")
	}
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		os.Exit(3)
	}
	if os.Getenv("MNET_DESK_COORDINATOR") == "1" {
		c, err := xgb.NewConn()
		if err != nil {
			t.Fatal(err)
		}
		keyboard, err := deskKeyboardFingerprint(c)
		c.Close()
		if err != nil {
			t.Fatal(err)
		}
		command := deskFixtureCommand()
		command.Env = append(command.Env, "MNET_DESK_COORDINATOR=0")
		p, err := newInputProcessWithInit(command, &attendedProcessCommand{Kind: "desk-init", Keyboard: keyboard})
		if err != nil {
			t.Fatal(err)
		}
		input := &deskProcessInput{p, func() bool { return true }}
		if err = input.Apply(desk.Input{Sequence: 1, Kind: "key", Action: "down", Code: 38}); err != nil {
			t.Fatal(err)
		}
		if err = input.Apply(desk.Input{Sequence: 2, Kind: "button", Action: "down", Code: 1}); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(p.cmd.Process.Pid)
		if err = os.WriteFile(os.Getenv("MNET_DESK_READY"), data, 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	err := runDeskProcess(os.NewFile(3, "commands"), os.NewFile(4, "replies"), attendedProcessLease)
	if err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func TestDeskX11ChildSurvivesCoordinatorLoss(t *testing.T) {
	if os.Getenv("MNET_DESK_NATIVE_TEST") != "1" {
		t.Skip("requires disposable X")
	}
	observer, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	root := xproto.Setup(observer).DefaultScreen(observer).Root
	for _, signal := range []syscall.Signal{syscall.SIGKILL, syscall.SIGSTOP} {
		t.Run(signal.String(), func(t *testing.T) {
			ready := filepath.Join(t.TempDir(), "ready.json")
			command := deskFixtureCommand()
			command.Env = append(command.Env, "MNET_DESK_COORDINATOR=1", "MNET_DESK_READY="+ready)
			if err = command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { command.Process.Kill(); command.Wait() }()
			var child int
			until := time.Now().Add(4 * time.Second)
			for {
				data, readErr := os.ReadFile(ready)
				if readErr == nil && json.Unmarshal(data, &child) == nil {
					break
				}
				if time.Now().After(until) {
					t.Fatal("coordinator never ready")
				}
				time.Sleep(time.Millisecond)
			}
			defer syscall.Kill(child, syscall.SIGKILL)
			keys, e := xproto.QueryKeymap(observer).Reply()
			pointer, pe := xproto.QueryPointer(observer, root).Reply()
			if e != nil || pe != nil || keys.Keys[38/8]&(1<<(38%8)) == 0 || pointer.Mask&xproto.ButtonMask1 == 0 {
				t.Fatal("physical fixture not held before loss")
			}
			if err = command.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			until = time.Now().Add(3 * time.Second)
			for {
				keys, e = xproto.QueryKeymap(observer).Reply()
				pointer, pe = xproto.QueryPointer(observer, root).Reply()
				if e == nil && pe == nil && keys.Keys[38/8]&(1<<(38%8)) == 0 && pointer.Mask&xproto.ButtonMask1 == 0 {
					break
				}
				if time.Now().After(until) {
					t.Fatal("coordinator loss left physical keys/buttons held")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}
