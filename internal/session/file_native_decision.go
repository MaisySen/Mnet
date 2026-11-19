package session

import (
	"errors"
	"sync"
	"time"
)

// Private bridge preparation. Native presentation never reconstructs a gate
// request from metadata. No mobile/public constructor or callback route exists.
type nativeFileDecision struct {
	mu               sync.Mutex
	gate             *FileApprovalGate
	request          FileApprovalRequest
	staging          string
	selected         *fileExportSelection
	resolved, closed bool
	watched          chan struct{}
}

var errNativeFileDecision = errors.New("local file decision unavailable")

func newNativeFileDecision(gate *FileApprovalGate, request FileApprovalRequest, staging string) (*nativeFileDecision, error) {
	if gate == nil {
		return nil, errNativeFileDecision
	}
	gate.mu.Lock()
	pending := gate.pending[request.ID]
	valid := !gate.closed && pending != nil && pending.request == request && time.Now().Before(pending.deadline)
	gate.mu.Unlock()
	if !valid {
		return nil, errNativeFileDecision
	}
	decision := &nativeFileDecision{gate: gate, request: request, staging: staging, watched: make(chan struct{})}
	go func() {
		defer close(decision.watched)
		<-request.Done
		decision.mu.Lock()
		// Decide closes Done on successful handoff too. Choose holds this lock
		// through that call, so a successful selected output is not canceled here.
		if !decision.resolved {
			decision.resolved = true
		}
		decision.mu.Unlock()
	}()
	return decision, nil
}
func (d *nativeFileDecision) current() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.resolved {
		return false
	}
	select {
	case <-d.request.Done:
		return false
	default:
	}
	return time.Now().Before(d.request.Deadline)
}
func (d *nativeFileDecision) choose(output nativeFileOutput) error {
	if output == nil {
		return errNativeFileDecision
	}
	adapter := newNativeFileExport(output)
	selection := newFileExportSelection(adapter)
	if d == nil {
		selection.release()
		return errNativeFileDecision
	}
	d.mu.Lock()
	if d.closed || d.resolved {
		d.mu.Unlock()
		selection.release()
		return errNativeFileDecision
	}
	// Gate retains the original exact request and is authoritative for expiry,
	// session/gate closure and one-use state. No external callback is run here.
	err := d.gate.decideExport(d.request, d.staging, selection)
	if err == nil {
		d.resolved = true
		d.selected = selection
	}
	d.mu.Unlock()
	if err != nil {
		selection.release()
		return errNativeFileDecision
	}
	return nil
}
func (d *nativeFileDecision) decline() error {
	if d == nil {
		return errNativeFileDecision
	}
	d.mu.Lock()
	if d.closed || d.resolved {
		d.mu.Unlock()
		return errNativeFileDecision
	}
	err := d.gate.Decide(d.request, false, "")
	if err == nil {
		d.resolved = true
	}
	d.mu.Unlock()
	if err != nil {
		return errNativeFileDecision
	}
	return nil
}

// close is the local owner teardown, distinct from successful prompt Done.
// Selection release is one-use with the receiver, including after completion.
func (d *nativeFileDecision) close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	selected := d.selected
	if !d.resolved {
		_ = d.gate.Decide(d.request, false, "")
		d.resolved = true
	}
	d.mu.Unlock()
	selected.release()
	<-d.watched
}
