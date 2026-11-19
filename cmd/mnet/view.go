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
		return err
	}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		// Local close and display failure are checked before each completed frame,
		// so a continuously full frame mailbox cannot starve local End.
		if end, err := display.Poll(); err != nil {
			return err
		} else if end {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case r := <-result:
			joined = true
			if r.err != nil {
				return r.err
			}
			if r.mux == nil {
				return errors.New("viewer transport unavailable")
			}
			mux = r.mux
			closed = mux.Closed()
			control := session.NewViewerControl(nil)
			defer control.End()
			session.ConnectViewerWithOptions(mux, session.ViewerOptions{Control: control, OnFrame: func(fr session.ScreenFrame) {
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
				return err
			}
		case <-closed:
			return errors.New("desktop viewing ended: host or path closed")
		case frame := <-frames:
			// Cancellation wins over queued content; no retired run can reanimate pixels.
			if ctx.Err() != nil {
				return nil
			}
			select {
			case <-closed:
				return errors.New("desktop viewing ended: host or path closed")
			default:
			}
			if err := display.Frame(frame); err != nil {
				return err
			}
		case <-tick.C:
			end, err := display.Poll()
			if err != nil {
				return err
			}
			if end {
				return nil
			}
		}
	}
}

func sessionView(peer string) {
	display, err := newDesktopView(peer)
	if err != nil {
		fatal(err.Error())
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err = runDesktopView(ctx, display, func(ctx context.Context) (*session.Mux, error) {
		access, name, err := prepareDesktopView(ctx, peer)
		if err != nil {
			return nil, err
		}
		return dialSessionTryAuthorizedContext(ctx, access, name)
	})
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
