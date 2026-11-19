package session

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Approval requests expire before the existing sender's30-second accept wait.
const maxFileApprovalWait = 20 * time.Second
const maxPendingFileApprovals = 8

// FileApprovalRequest is local-only, never a wire permission. A chooser must
// display Offer as untrusted metadata, select a local directory, and return the
// unmodified request to Decide. Done retires the prompt on answer/cancellation.
// This is not a Multies identity assertion or a completed native chooser.
type FileApprovalRequest struct {
	ID       string          `json:"-"`
	Session  string          `json:"-"`
	StreamID uint32          `json:"-"`
	Deadline time.Time       `json:"-"`
	Offer    FileOffer       `json:"-"`
	Done     <-chan struct{} `json:"-"`
}

// Avoid accidental logging of correlation handles or untrusted offer metadata.
func (FileApprovalRequest) String() string   { return "local file approval request" }
func (FileApprovalRequest) GoString() string { return "local file approval request" }

type fileApprovalPending struct {
	request  FileApprovalRequest
	deadline time.Time
	done     chan struct{}
	result   chan fileReceiveDestination // empty directory means decline
}

// FileApprovalGate binds once to one mux and bounds both pending decisions and
// queued UI notifications. A new session requires a new gate. It has no UI or
// storage effects; only a local caller of Decide can select a destination.
// The zero value is closed/unavailable. Retired queued notifications may remain
// until drained; consumers must check Done before presenting them.
type FileApprovalGate struct {
	exportBudget fileExportBudget
	mu           sync.Mutex
	session      string
	bound        *Mux
	timeout      time.Duration
	requests     chan FileApprovalRequest
	pending      map[string]*fileApprovalPending
	closed       bool
}

// NewFileApprovalGate limits waits to20seconds; nonpositive uses that maximum.
func NewFileApprovalGate(timeout time.Duration) *FileApprovalGate {
	if timeout <= 0 || timeout > maxFileApprovalWait {
		timeout = maxFileApprovalWait
	}
	return &FileApprovalGate{session: controlToken(), timeout: timeout, requests: make(chan FileApprovalRequest, maxPendingFileApprovals), pending: make(map[string]*fileApprovalPending)}
}
func (g *FileApprovalGate) Requests() <-chan FileApprovalRequest {
	if g == nil {
		return nil
	}
	return g.requests
}
func (g *FileApprovalGate) begin(s Stream, offer FileOffer) (*fileApprovalPending, bool) {
	if g == nil {
		return nil, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.session == "" || g.requests == nil || len(g.pending) >= maxPendingFileApprovals || (g.bound != nil && g.bound != s.Mux) {
		return nil, false
	}
	select {
	case <-s.Mux.Closed():
		return nil, false
	default:
	}
	id := controlToken()
	if id == "" || g.pending[id] != nil {
		return nil, false
	}
	done := make(chan struct{})
	p := &fileApprovalPending{deadline: time.Now().Add(g.timeout), done: done, result: make(chan fileReceiveDestination, 1)}
	p.request = FileApprovalRequest{ID: id, Session: g.session, StreamID: s.ID, Deadline: p.deadline, Offer: offer, Done: done}
	select {
	case g.requests <- p.request:
		g.bound = s.Mux
		g.pending[id] = p
		return p, true
	default:
		return nil, false
	}
}
func (g *FileApprovalGate) retire(p *fileApprovalPending) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending[p.request.ID] == p {
		delete(g.pending, p.request.ID)
		close(p.done)
	}
}

// Decide is a one-use local answer for the exact immutable request. A malformed
// answer cannot consume another prompt. Approval requires an absolute local
// directory, not a peer-selected path. No directory is created here.
func (g *FileApprovalGate) Decide(request FileApprovalRequest, allow bool, dir string) error {
	return g.decideDestination(request, allow, fileReceiveDestination{dir: dir})
}

// Internal-only opt-in for future native adapters. No consumer entry point or
// external/native API can construct this capability yet.
func (g *FileApprovalGate) decideExport(request FileApprovalRequest, staging string, selection *fileExportSelection) error {
	if selection == nil {
		return errors.New("export selection unavailable")
	}
	return g.decideDestination(request, true, fileReceiveDestination{dir: staging, export: selection})
}
func (g *FileApprovalGate) decideDestination(request FileApprovalRequest, allow bool, destination fileReceiveDestination) error {

	if g == nil {
		return errors.New("file approval unavailable")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.pending[request.ID]
	if g.closed || p == nil || p.request != request {
		return errors.New("stale file approval")
	}
	select {
	case <-g.bound.Closed():
		delete(g.pending, request.ID)
		close(p.done)
		return errors.New("stale file approval")
	default:
	}
	if !time.Now().Before(p.deadline) {
		delete(g.pending, request.ID)
		close(p.done)
		return errors.New("expired file approval")
	}
	if allow && (strings.TrimSpace(destination.dir) == "" || !filepath.IsAbs(destination.dir)) {
		return errors.New("file approval needs an absolute local directory")
	}
	if allow && destination.export != nil && !destination.export.claim(request, &g.exportBudget) {
		return errors.New("export selection already consumed or unavailable")
	}
	if !allow {
		destination = fileReceiveDestination{}
	}
	delete(g.pending, request.ID)
	p.result <- destination
	close(p.done)
	return nil
}

// Close retires all prompts; queued requests also have Done closed. Idempotent.
func (g *FileApprovalGate) Close() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	for id, p := range g.pending {
		delete(g.pending, id)
		close(p.done)
	}
	if g.requests != nil {
		close(g.requests)
	}
}

// approvedDestination waits on local state, not an arbitrary blocking UI callback.
// Peer data/FIN/reset before acceptance is abandonment/protocol misuse. Never
// consume or buffer speculative file bytes on behalf of a pending approval.
func (r FileReceiver) approvedDestination(s Stream, offer FileOffer, allowed func() bool) (fileReceiveDestination, bool) {
	p, ok := r.Approval.begin(s, offer)
	if !ok {
		return fileReceiveDestination{}, false
	}
	var selected *fileExportSelection
	transferred := false
	defer func() {
		// Synchronize with Decide before draining. A successful queued decision
		// may lose a select race to End/reset/speculative data before consumption.
		r.Approval.retire(p)
		if !transferred {
			selected.release()
			select {
			case abandoned := <-p.result:
				abandoned.export.release()
			default:
			}
		}
	}()
	timer := time.NewTimer(time.Until(p.deadline))
	defer timer.Stop()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	var released <-chan struct{}
	if s.state != nil {
		released = s.state.done
	}

	finish := func(dir fileReceiveDestination) (fileReceiveDestination, bool) {
		selected = dir.export
		if dir.dir == "" || !allowed() || !time.Now().Before(p.deadline) {
			return fileReceiveDestination{}, false
		}
		select {
		case <-released:
			return fileReceiveDestination{}, false
		case f, ok := <-s.in:
			if ok {
				s.Mux.releaseInbound(&f)
			}
			return fileReceiveDestination{}, false
		default:
			transferred = true
			return dir, true
		}
	}
	for {
		select {
		case <-p.done:
			// Decide queues the result before closing Done. Retiring has no result.
			select {
			case dir := <-p.result:
				return finish(dir)
			default:
				return fileReceiveDestination{}, false
			}
		case f, ok := <-s.in:
			if ok {
				s.Mux.releaseInbound(&f)
			}
			return fileReceiveDestination{}, false
		case <-released:
			return fileReceiveDestination{}, false
		case <-s.Mux.Closed():
			return fileReceiveDestination{}, false
		case <-timer.C:
			return fileReceiveDestination{}, false
		case <-poll.C:
			if !allowed() {
				return fileReceiveDestination{}, false
			}
		}
	}
}
