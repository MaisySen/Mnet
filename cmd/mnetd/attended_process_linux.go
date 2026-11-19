//go:build linux

package main

// This process owner is restricted to the explicit attended engineering mode.
// It covers coordinator loss while this local injector and X server survive,
// never general process-death, physical ownership or OS-wide isolation.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"fintex.au/mnet/internal/session"
	"github.com/jezek/xgb/xproto"
)

const attendedProcessArg = "--mnet-private-attended-input-v1"
const attendedProcessLease = 2 * time.Second

type attendedProcessCommand struct {
	Kind  string
	ID    uint64
	Event session.InputEvent
}
type attendedProcessReply struct {
	Kind          string
	ID            uint64
	OK            bool
	Width, Height int
}

// Only fixed inherited descriptors cross this boundary. No TCP/socket path,
// environment-selected executable, remote peer label or shell is involved.
func runAttendedProcess(in io.Reader, out io.Writer, lease time.Duration) (result error) {
	if lease <= 0 {
		return errors.New("invalid local input lease")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner, err := claimAttendedInjectorOwner(cancel)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, owner.Close()) }()
	backend, err := newAttendedX11Injector()
	if err != nil {
		return errors.New("local input unavailable")
	}
	input := backend.(*attendedX11Injector)
	defer func() { result = errors.Join(result, input.Close()) }()
	commands := make(chan attendedProcessCommand, 1)
	pulse := make(chan struct{}, 1)
	go func() {
		defer cancel()
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 1024), 32768)
		for scanner.Scan() {
			var c attendedProcessCommand
			if json.Unmarshal(scanner.Bytes(), &c) != nil {
				return
			}
			switch c.Kind {
			case "pulse":
				select {
				case pulse <- struct{}{}:
				default:
				}
			case "input":
				if c.ID == 0 {
					return
				}
				select {
				case commands <- c:
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
	if encoder.Encode(attendedProcessReply{Kind: "ready", OK: true, Width: input.host.w / input.host.scale, Height: input.host.h / input.host.scale}) != nil {
		return errors.New("local input reply unavailable")
	}
	var last uint64
	for {
		select {
		case <-ctx.Done():
			// Release only this injector's own ledger, never global pressed-state sweep.
			if err = input.Close(); err != nil {
				return errors.New("local input release failed")
			}
			return nil
		case command := <-commands:
			if command.ID <= last {
				return errors.New("retired local input request")
			}
			last = command.ID
			live := func() bool { return ctx.Err() == nil }
			ev := command.Event
			if ev.Kind == "pointer" && (ev.Action == "down" || ev.Action == "click" || ev.Action == "up") {
				button := buttonCode(ev.Button)
				// A release without a recorded press is never used to fix guessed state.
				if ev.Action == "up" && !input.host.pressed[button] {
					err = errors.New("unowned release")
				} else if ev.Action != "up" {
					p, e := xproto.QueryPointer(input.host.conn, input.host.root).Reply()
					if e != nil || p.Mask&(uint16(1)<<uint(button+7)) != 0 {
						err = errors.New("preheld input refused")
					} else {
						err = nil
					}
				} else {
					err = nil
				}
			} else {
				err = nil
			}
			if err == nil {
				err = input.InjectAuthorized(ev, live)
			}
			ok := err == nil
			if encoder.Encode(attendedProcessReply{Kind: "input", ID: command.ID, OK: ok}) != nil {
				return errors.New("local input reply unavailable")
			}
			if !ok {
				return errors.New("local input refused")
			}
		}
	}
}

func maybeRunAttendedProcess() bool {
	if len(os.Args) != 2 || os.Args[1] != attendedProcessArg {
		return false
	}
	in := os.NewFile(3, "attended-input")
	out := os.NewFile(4, "attended-reply")
	if in == nil || out == nil {
		os.Exit(2)
	}
	err := runAttendedProcess(in, out, attendedProcessLease)
	_ = in.Close()
	_ = out.Close()
	if err != nil {
		os.Exit(2)
	}
	os.Exit(0)
	return true
}

type attendedProcessInjector struct {
	width, height int
	writer        *os.File
	replies       chan attendedProcessReply
	exit          chan struct{}
	cmd           *exec.Cmd
	writeMu       sync.Mutex
	eventMu       sync.Mutex
	closeOnce     sync.Once
	terminal      chan struct{}
	id            uint64
	err           error
}

func newAttendedProcessInjector(command *exec.Cmd) (*attendedProcessInjector, error) {
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		inRead.Close()
		inWrite.Close()
		return nil, err
	}
	command.ExtraFiles = []*os.File{inRead, outWrite}
	command.Stderr = io.Discard
	if err = command.Start(); err != nil {
		inRead.Close()
		inWrite.Close()
		outRead.Close()
		outWrite.Close()
		return nil, err
	}
	inRead.Close()
	outWrite.Close()
	p := &attendedProcessInjector{writer: inWrite, replies: make(chan attendedProcessReply, 2), exit: make(chan struct{}), terminal: make(chan struct{}), cmd: command}
	go func() { p.err = command.Wait(); close(p.exit); p.retire() }()
	go func() {
		defer outRead.Close()
		scanner := bufio.NewScanner(outRead)
		scanner.Buffer(make([]byte, 256), 1024)
		for scanner.Scan() {
			var r attendedProcessReply
			if json.Unmarshal(scanner.Bytes(), &r) != nil {
				return
			}
			select {
			case p.replies <- r:
			case <-p.exit:
				return
			}
		}
	}()
	select {
	case r := <-p.replies:
		p.width, p.height = r.Width, r.Height
		if r.Kind != "ready" || !r.OK || r.Width <= 0 || r.Height <= 0 {
			p.Close()
			return nil, errors.New("local injector not ready")
		}
	case <-p.exit:
		p.Close()
		return nil, errors.New("local injector exited before ready")
	case <-time.After(2 * time.Second):
		p.Close()
		return nil, errors.New("local injector readiness timed out")
	}
	go func() {
		timer := time.NewTicker(500 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-p.terminal:
				return
			case <-p.exit:
				return
			case <-timer.C:
				if p.send(attendedProcessCommand{Kind: "pulse"}) != nil {
					p.retire()
					return
				}
			}
		}
	}()
	return p, nil
}
func (p *attendedProcessInjector) send(c attendedProcessCommand) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	select {
	case <-p.terminal:
		return errors.New("local input retired")
	default:
	}
	if err := p.writer.SetWriteDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
		return err
	}
	return json.NewEncoder(p.writer).Encode(c)
}
func (p *attendedProcessInjector) retire() {
	p.closeOnce.Do(func() { close(p.terminal); _ = p.writer.Close() })
}
func (p *attendedProcessInjector) Close() error {
	p.retire()
	select {
	case <-p.exit:
		return p.err
	case <-time.After(3 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.exit
		return errors.New("local injector did not confirm release")
	}
}
func (p *attendedProcessInjector) Inject(session.InputEvent) error {
	return errors.New("attended input requires exact authorization")
}
func (p *attendedProcessInjector) InjectAuthorized(ev session.InputEvent, live func() bool) error {
	p.eventMu.Lock()
	defer p.eventMu.Unlock()
	if len(ev.Text) > 4096 {
		p.retire()
		return errors.New("local text command too large")
	}
	if live == nil || !live() {
		p.retire()
		return errors.New("retired input owner")
	}
	p.id++
	if p.send(attendedProcessCommand{Kind: "input", ID: p.id, Event: ev}) != nil {
		p.retire()
		return errors.New("local input command failed")
	}
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case r := <-p.replies:
			if r.Kind != "input" || r.ID != p.id || !r.OK {
				p.retire()
				return errors.New("local input refused or retired")
			}
			return nil
		case <-tick.C:
			if !live() {
				p.retire()
				return errors.New("in-flight input owner retired")
			}
		case <-p.exit:
			return errors.New("local input process exited")
		case <-p.terminal:
			return errors.New("local input process retired")
		case <-timeout.C:
			p.retire()
			return errors.New("local input command timed out")
		}
	}
}

func (p *attendedProcessInjector) CheckCaptureGeometry(width, height int) error {
	if width != p.width || height != p.height {
		return errors.New("attended input/capture geometry differs")
	}
	return nil
}
func startAttendedProcessInjector() (session.Injector, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("local injector executable unavailable")
	}
	return startAttendedProcessCommand(exec.Command(executable, attendedProcessArg))
}

func (p *attendedProcessInjector) Stopped() <-chan struct{} { return p.terminal }

func startAttendedProcessCommand(command *exec.Cmd) (session.Injector, error) {
	process, err := newAttendedProcessInjector(command)
	if err != nil {
		return nil, err
	}
	return process, nil
}
