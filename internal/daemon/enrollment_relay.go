package daemon

// Private finite native relay preparation. No Up/relayLoop/production caller,
// retry, legacy rejoin, map RelayURL relaxation or automatic bind installation.
import (
	"context"
	"fintex.au/mnet/internal/bind"
	"fintex.au/mnet/internal/controlclient"
)

type nativeRegistrarRelay struct {
	daemon   *Daemon
	socket   *controlclient.RegistrarRelay
	revision [32]byte
	ctx      context.Context
	bind     *bind.Bind
}

func (r *nativeRegistrarRelay) Close() {
	if r != nil && r.socket != nil {
		r.socket.Close()
	}
}
func (r *nativeRegistrarRelay) Done() <-chan struct{} { return r.socket.Done() }
func (r *nativeRegistrarRelay) current() bool {
	d := r.daemon
	if !d.deviceMu.TryRLock() {
		return false
	}
	same := d.dev != nil && d.bind == r.bind
	d.deviceMu.RUnlock()
	if !same {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.checkNativeCurrentAfterLocked(r.ctx, r.revision) == nil
}
func (r *nativeRegistrarRelay) Send(dst string, packet []byte) error {
	if r == nil || r.socket == nil || !r.current() {
		return errEnrollmentContextStale
	}
	return r.socket.Send(dst, packet)
}
func (d *Daemon) nativeCurrentRelay(ctx context.Context, limits controlclient.RegistrarLimits, maxFrames int) (*nativeRegistrarRelay, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errEnrollmentContextStale
	}
	d.deviceMu.Lock()
	b := d.bind
	running := d.dev != nil && b != nil
	d.deviceMu.Unlock()
	if !running {
		return nil, errEnrollmentContextStale
	}
	d.mu.Lock()
	client, rev, err := d.nativeClientLocked(limits)
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	owner := &nativeRegistrarRelay{daemon: d, revision: rev, ctx: ctx, bind: b}
	socket, err := client.DialRegistrarRelay(ctx, maxFrames, func(src string, packet []byte) {
		// Never re-read d.bind after authorization: an older socket can only feed
		// its captured generation; that Bind rejects injection once it is closed.
		if owner.current() {
			b.Inject(src, packet)
		}
	})
	if err != nil {
		return nil, err
	}
	owner.socket = socket
	if !owner.current() {
		socket.Close()
		return nil, errEnrollmentContextStale
	}
	return owner, nil
}
