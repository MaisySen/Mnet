//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"fintex.au/mnet/internal/desk"
)

const deskProcessArg = "--mnet-private-desk-input-v1"

// A separate mode cannot reinterpret an attended text grant as a Desk pair.
// Both use the local transport mechanics and cooperating desktop owner locks.
func runDeskProcess(in io.Reader, out io.Writer, lease time.Duration) (result error) {
	if lease <= 0 {
		return errors.New("invalid desk lease")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	commands := make(chan attendedProcessCommand, 1)
	pulse := make(chan struct{}, 1)
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 512), 4096)
	if !scanner.Scan() {
		return errors.New("desk initialization missing")
	}
	var init attendedProcessCommand
	if json.Unmarshal(scanner.Bytes(), &init) != nil || init.Kind != "desk-init" || len(init.Keyboard) != 64 || init.DeskInput != nil {
		return errors.New("invalid desk initialization")
	}
	native, err := newDeskNativeInput(init.Keyboard, func() bool { return ctx.Err() == nil })
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, native.Release()) }()
	recipient, err := desk.NewRecipient(native, native.width, native.height, func() bool { return ctx.Err() == nil })
	if err != nil {
		return err
	}
	go func() {
		defer cancel()
		for scanner.Scan() {
			var command attendedProcessCommand
			if json.Unmarshal(scanner.Bytes(), &command) != nil {
				return
			}
			switch command.Kind {
			case "pulse":
				if command.DeskInput != nil || command.Keyboard != "" {
					return
				}
				select {
				case pulse <- struct{}{}:
				default:
				}
			case "desk-input":
				if command.ID == 0 || command.DeskInput == nil || command.Keyboard != "" {
					return
				}
				select {
				case commands <- command:
				case <-ctx.Done():
					return
				default:
					return
				}
			default:
				return
			}
		}
	}()
	go func() {
		timer := time.NewTimer(lease)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-pulse:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(lease)
			case <-timer.C:
				cancel()
				return
			}
		}
	}()
	encoder := json.NewEncoder(out)
	if encoder.Encode(attendedProcessReply{Kind: "ready", OK: true, Width: int(native.width), Height: int(native.height)}) != nil {
		return errors.New("desk readiness failed")
	}
	var last uint64
	for {
		select {
		case <-ctx.Done():
			return recipient.Close()
		case <-native.done:
			return errors.New("native desk input retired")
		case command := <-commands:
			if command.ID <= last {
				return errors.New("stale desk process request")
			}
			last = command.ID
			err = recipient.Apply(*command.DeskInput)
			if encoder.Encode(attendedProcessReply{Kind: "desk-input", ID: command.ID, OK: err == nil}) != nil {
				return errors.New("desk input reply failed")
			}
			if err != nil {
				return err
			}
		}
	}
}

type deskProcessInput struct {
	process *attendedProcessInjector
	live    func() bool
}

func startDeskProcessInput(keyboard string, live func() bool) (*deskProcessInput, error) {
	if live == nil || !live() {
		return nil, errors.New("desk pair retired")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	p, err := newInputProcessWithInit(exec.Command(executable, deskProcessArg), &attendedProcessCommand{Kind: "desk-init", Keyboard: keyboard})
	if err != nil {
		return nil, err
	}
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-p.terminal:
				return
			case <-tick.C:
				if !live() {
					p.retire()
					return
				}
			}
		}
	}()
	return &deskProcessInput{p, live}, nil
}
func (p *deskProcessInput) Release() error { return p.process.Close() }
func (p *deskProcessInput) Apply(ev desk.Input) error {
	c := p.process
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	if !p.live() {
		c.retire()
		return errors.New("desk pair retired")
	}
	c.id++
	if err := c.send(attendedProcessCommand{Kind: "desk-input", ID: c.id, DeskInput: &ev}); err != nil {
		c.retire()
		return err
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case reply := <-c.replies:
			if reply.Kind != "desk-input" || reply.ID != c.id || !reply.OK {
				c.retire()
				return errors.New("desk native input refused")
			}
			return nil
		case <-ticker.C:
			if !p.live() {
				c.retire()
				return errors.New("desk pair lost during native input")
			}
		case <-c.terminal:
			return errors.New("desk child retired")
		case <-timeout.C:
			c.retire()
			return errors.New("desk child timeout")
		}
	}
}
