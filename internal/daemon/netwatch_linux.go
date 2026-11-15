//go:build linux

package daemon

// Roaming support: subscribe to kernel link/address events over
// rtnetlink so the daemon re-resolves its endpoints as soon as the
// machine changes networks, instead of waiting out the 45 second
// discovery interval. Events are debounced: the first event starts a
// 2 second settle window, then one signal fires.

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// watchNetworkChanges returns a channel that receives one signal per
// settled network change, or nil when rtnetlink is unavailable.
func watchNetworkChanges(ctx context.Context) <-chan struct{} {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM, unix.NETLINK_ROUTE)
	if err != nil {
		return nil
	}
	sa := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR,
	}
	if err := unix.Bind(fd, sa); err != nil {
		unix.Close(fd)
		return nil
	}
	ch := make(chan struct{}, 1)
	var closed atomic.Bool
	go func() {
		<-ctx.Done()
		closed.Store(true)
		unix.Close(fd) // unblocks the reader
	}()
	go func() {
		defer close(ch)
		buf := make([]byte, 8192)
		var timer *time.Timer
		var timerC <-chan time.Time
		for {
			// Poll with a short deadline so a pending debounce timer
			// can fire without a further event.
			tv := unix.NsecToTimeval(300 * 1e6)
			unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
			n, err := unix.Read(fd, buf)
			switch {
			case err == nil && n > 0:
				if timerC == nil {
					timer = time.NewTimer(2 * time.Second)
					timerC = timer.C
				}
			case closed.Load():
				if timer != nil {
					timer.Stop()
				}
				return
			default:
				// timeout: deliver a settled signal if one is due
				if timerC != nil {
					select {
					case <-timerC:
						select {
						case ch <- struct{}{}:
						default:
						}
						timerC = nil
					default:
					}
				}
			}
		}
	}()
	return ch
}
