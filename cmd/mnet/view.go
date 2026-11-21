package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fintex.au/mnet/internal/daemon"
	"fintex.au/mnet/internal/session"
)

type desktopView interface {
	Frame(session.ScreenFrame) error
	State(string) error
	Poll() (bool, error)
	Close() error
}

type viewDialResult struct {
	mux *session.Mux
	err error
}

// One attempt only. A closed window retires pending hello and current session;
// this path never reconnects or carries approval to another transport.
func runDesktopView(ctx context.Context, display desktopView, dial func(context.Context) (*session.Mux, error)) (runErr error) {
	return runDesktopWindow(ctx, display, dial, false)
}

var errDesktopFirstScreen = errors.New("no usable screen arrived within 15 seconds; session ended")

// The host or the path ended an admitted viewing session. Wrapping keeps the
// established CLI text while letting the home report the specific outcome.
var errDesktopHostEnded = errors.New("desktop viewing ended")

// sessionRefusedPrefix marks the attended host's admission refusal produced by
// internal/session hello handling; the home maps it to a specific outcome.
const sessionRefusedPrefix = "session: refused:"

func desktopViewRefused(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), sessionRefusedPrefix)
}

func runDesktopWindow(ctx context.Context, display desktopView, dial func(context.Context) (*session.Mux, error), pointer bool) error {
	_, err := runDesktopWindowWithFirstScreenWait(ctx, display, dial, pointer, 15*time.Second)
	return err
}

// The first-screen bound starts after host admission, not while the person is
// deciding whether to allow viewing. It never retries a transport or approval.
func runDesktopWindowWithFirstScreenWait(ctx context.Context, display desktopView, dial func(context.Context) (*session.Mux, error), pointer bool, firstScreenWait time.Duration) (viewed bool, runErr error) {
	var firstScreenTimer *time.Timer
	var firstScreenDeadline <-chan time.Time
	defer func() {
		if firstScreenTimer != nil {
			firstScreenTimer.Stop()
		}
	}()
	var pointerView desktopPointerView
	if pointer {
		var ok bool
		pointerView, ok = display.(desktopPointerView)
		if !ok {
			return false, errors.Join(errors.New("native pointer UI unavailable"), display.Close())
		}
	}
	var control *session.ViewerControl
	var viewer *session.Viewer
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { runErr = errors.Join(runErr, display.Close()) }()
	frames := make(chan session.ScreenFrame, 1)
	result := make(chan viewDialResult, 1)
	go func() { m, err := dial(ctx); result <- viewDialResult{m, err} }()
	var mux *session.Mux
	var closed <-chan struct{}
	joined := false
	defer func() {
		cancel()
		if !joined {
			r := <-result
			mux = r.mux
		}
		if mux != nil {
			mux.Close()
		}
	}()
	if err := display.State("Awaiting host approval. View only. Escape ends."); err != nil {
		return false, err
	}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		// Local close and display failure are checked before each completed frame,
		// so a continuously full frame mailbox cannot starve local End.
		if end, err := display.Poll(); err != nil {
			return viewed, err
		} else if end {
			return viewed, nil
		}
		if pointer {
			if err := dispatchDesktopPointer(pointerView, control, viewer); err != nil {
				return viewed, err
			}
		}
		select {
		case <-ctx.Done():
			return viewed, nil
		case <-firstScreenDeadline:
			if ctx.Err() != nil {
				return viewed, nil
			}
			return viewed, errDesktopFirstScreen
		case r := <-result:
			joined = true
			// A canceled owner can close its transport before this select
			// chooses between the simultaneously ready completion channels.
			mux = r.mux
			if ctx.Err() != nil {
				return viewed, nil
			}
			if r.err != nil {
				return viewed, r.err
			}
			if r.mux == nil {
				return viewed, errors.New("viewer transport unavailable")
			}
			mux = r.mux
			closed = mux.Closed()
			firstScreenTimer = time.NewTimer(firstScreenWait)
			firstScreenDeadline = firstScreenTimer.C
			control = session.NewViewerControl(nil)
			defer control.End()
			viewer = session.ConnectViewerWithOptions(mux, session.ViewerOptions{Control: control, OnFrame: func(fr session.ScreenFrame) {
				if ctx.Err() != nil {
					return
				}
				select {
				case frames <- fr:
				default:
					select {
					case <-frames:
					default:
					}
					select {
					case frames <- fr:
					default:
					}
				}
			}})
			if err := display.State("Connected. Waiting for screen. View only. Escape ends."); err != nil {
				return viewed, err
			}
		case <-closed:
			if ctx.Err() != nil {
				return viewed, nil
			}
			return viewed, fmt.Errorf("%w: host or path closed", errDesktopHostEnded)
		case frame := <-frames:
			// Cancellation wins over queued content; no retired run can reanimate pixels.
			if ctx.Err() != nil {
				return viewed, nil
			}
			select {
			case <-closed:
				if ctx.Err() != nil {
					return viewed, nil
				}
				return viewed, fmt.Errorf("%w: host or path closed", errDesktopHostEnded)
			default:
			}
			if err := display.Frame(frame); err != nil {
				return viewed, err
			}
			viewed = true
			if firstScreenTimer != nil {
				firstScreenTimer.Stop()
				firstScreenDeadline = nil
			}
		case <-tick.C:
			end, err := display.Poll()
			if err != nil {
				return viewed, err
			}
			if end {
				return viewed, nil
			}
		}
	}
}

func sessionView(peer string)  { sessionDesktop(peer, false) }
func sessionPoint(peer string) { sessionDesktop(peer, true) }
func sessionDesktop(peer string, pointer bool) {
	var display desktopView
	var err error
	if pointer {
		display, err = newDesktopPointer(peer)
	} else {
		display, err = newDesktopView(peer)
	}
	if err != nil {
		fatal(err.Error())
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err = runDesktopWindow(ctx, display, func(ctx context.Context) (*session.Mux, error) {
		access, name, err := prepareDesktopView(ctx, peer)
		if err != nil {
			return nil, err
		}
		return dialSessionTryAuthorizedContext(ctx, access, name)
	}, pointer)
	if err != nil {
		fatal(err.Error())
	}
}

// Unlike the legacy terminal status helper, this path returns errors instead
// of exiting the process, and window cancellation reaches local API I/O.
func prepareDesktopView(ctx context.Context, peer string) (*cliSessionAuthorization, string, error) {
	hc := client(stateDir)
	read := func(path string, out any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase()+path, nil)
		if err != nil {
			return err
		}
		resp, err := hc.Do(req)
		if err != nil {
			return fmt.Errorf("local viewing authorization unavailable: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return errors.New("local viewing authorization refused")
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		if err != nil || len(data) > 4<<20 {
			return errors.New("invalid local viewing evidence")
		}
		if err = json.Unmarshal(data, out); err != nil {
			return errors.New("invalid local viewing evidence")
		}
		return nil
	}
	var st daemon.Status
	if err := read("/localapi/status", &st); err != nil {
		return nil, "", err
	}
	if st.Status != "up" {
		return nil, "", errors.New("network is not up")
	}
	var peers []daemon.SessionPeer
	if err := read("/localapi/session-peers", &peers); err != nil {
		return nil, "", err
	}
	pinned, err := selectCLISessionPeer(peer, &st, peers)
	if err != nil {
		return nil, "", err
	}
	lookup := func() ([]daemon.SessionPeer, error) { return readCLISessionPeers(hc) }
	return &cliSessionAuthorization{peer: pinned, lookup: lookup, requireAttended: true}, st.NodeName, nil
}

// The chooser makes no transport. Closing it cancels and joins the read; an
// explicit mode/peer choice then enters the existing one-attempt host journey.
func sessionBrowse() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if _, err := browseDesktopOnce(ctx, client(stateDir), apiBase()); err != nil {
		fatal(err.Error())
	}
}

// The viewed result lets the home distinguish a session that actually showed
// the host's screen from a chooser closed before any request.
func browseDesktopOnce(ctx context.Context, hc *http.Client, base string) (bool, error) {
	choice, err := chooseDesktopPeer(ctx, hc, base)
	if err != nil {
		return false, err
	}
	if choice == nil {
		return false, nil
	}
	var display desktopView
	if choice.Pointer {
		display, err = newDesktopPointer(choice.Peer.Name)
	} else {
		display, err = newDesktopView(choice.Peer.Name)
	}
	if err != nil {
		return false, err
	}
	return runDesktopWindowWithFirstScreenWait(ctx, display, func(ctx context.Context) (*session.Mux, error) {
		access, name, err := prepareDesktopChoice(ctx, *choice)
		if err != nil {
			return nil, err
		}
		return dialSessionTryAuthorizedContext(ctx, access, name)
	}, choice.Pointer, 15*time.Second)
}
