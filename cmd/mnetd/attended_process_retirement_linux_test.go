//go:build linux

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fintex.au/mnet/internal/session"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A protocol-only child deliberately withholds one ACK until its writer closes.
// No X server or native input is involved. This makes cancellation deterministic.
func TestAttendedProcessRetirementProtocolChild(t *testing.T) {
	if os.Getenv("MNET_RETIRE_PROTOCOL_CHILD") != "1" {
		t.Skip("owned child only")
	}
	in := os.NewFile(3, "commands")
	out := os.NewFile(4, "replies")
	if json.NewEncoder(out).Encode(attendedProcessReply{Kind: "ready", OK: true, Width: 16, Height: 16}) != nil {
		os.Exit(2)
	}
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		var command attendedProcessCommand
		if json.Unmarshal(scanner.Bytes(), &command) != nil {
			os.Exit(2)
		}
		if command.Kind == "input" {
			if os.WriteFile(os.Getenv("MNET_RETIRE_PROTOCOL_MARKER"), []byte("received"), 0600) != nil {
				os.Exit(2)
			}
		}
	}
	if scanner.Err() != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func TestAttendedProcessInFlightRetirementReturnsTypedErrorAndJoins(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "received")
	command := exec.Command(os.Args[0], "-test.run=^TestAttendedProcessRetirementProtocolChild$")
	command.Env = append(os.Environ(), "MNET_RETIRE_PROTOCOL_CHILD=1", "MNET_RETIRE_PROTOCOL_MARKER="+marker)
	p, err := newAttendedProcessInjector(command)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var live atomic.Bool
	live.Store(true)
	result := make(chan error, 1)
	go func() {
		result <- p.InjectAuthorized(session.InputEvent{Kind: "pointer", Action: "scroll", DY: -1}, live.Load)
	}()
	until := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(until) {
			t.Fatal("child did not receive in-flight command")
		}
		time.Sleep(time.Millisecond)
	}
	live.Store(false)
	select {
	case err := <-result:
		if !errors.Is(err, errAttendedInputRetired) {
			t.Fatal("retirement not typed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight cancellation blocked")
	}
	if err := p.Close(); err != nil {
		t.Fatal("expected protocol owner did not join", err)
	}
	select {
	case <-p.exit:
	default:
		t.Fatal("Close returned before child exit")
	}
}
