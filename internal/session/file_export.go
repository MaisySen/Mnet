package session

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// Internal-only destination seam. No production provider or native entry point
// implements it. Open owns the output on success, including every resource it
// returns. Methods must honor cancellation or eventually return; already-entered
// provider work cannot be forcibly rolled back by the receiver.
type fileExportCapability interface {
	Open(context.Context, FileOffer) (fileExportOutput, error)
	// Release retires selection-owned native resources even when Open never ran.
	// It is separate from output Abort and must be harmless after local success.
	Release()
}
type fileExportOutput interface {
	Write(context.Context, []byte) (int, error)
	// Finish reports only the adapter's observed local completion, not cloud
	// durability or atomic publication. Abort then releases remaining resources
	// without deleting successful output. On failure cleanup is best-effort.
	Finish(context.Context) error
	Abort()
}

type fileReceiveDestination struct {
	dir    string // path destination or local private staging root, never a URI
	export *fileExportSelection
}

// Selection is consumed by one exact approval. Reuse cannot authorize a second
// offer even when names/bytes match. It contains no remotely supplied identity.
const maxActiveFileExports = 8
const maxStagedFileExportBytes = DefaultMaxFileBytes

type fileExportBudget struct {
	mu     sync.Mutex
	active int
	bytes  int64
}

func (b *fileExportBudget) take(size int64) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if size < 0 || b.active >= maxActiveFileExports || size > maxStagedFileExportBytes-b.bytes {
		return false
	}
	b.active++
	b.bytes += size
	return true
}
func (b *fileExportBudget) release(size int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.active--
	b.bytes -= size
}

type fileExportSelection struct {
	mu         sync.Mutex
	capability fileExportCapability
	request    FileApprovalRequest
	budget     *fileExportBudget
	claimed    bool
	started    bool
	released   bool
}

func newFileExportSelection(capability fileExportCapability) *fileExportSelection {
	return &fileExportSelection{capability: capability}
}
func (s *fileExportSelection) release() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.released {
		s.mu.Unlock()
		return
	}
	s.released = true
	capability := s.capability
	s.mu.Unlock()
	if capability != nil {
		capability.Release()
	}
}
func (s *fileExportSelection) claim(request FileApprovalRequest, budget *fileExportBudget) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed || s.released || s.capability == nil {
		return false
	}
	s.claimed = true
	s.budget = budget
	s.request = request
	return true
}

type stagedExportTransaction struct {
	stage      *pathReceiveTransaction
	dir        string
	offer      FileOffer
	capability fileExportCapability
	ctx        context.Context
	cancel     context.CancelFunc
	watcher    chan struct{}
	allowed    func() bool
	used       bool
	budget     *fileExportBudget
	abortOnce  sync.Once
}

func beginExportReceive(destination fileReceiveDestination, offer FileOffer, closed, reset <-chan struct{}, allowed func() bool) (receiveTransaction, error) {
	selection := destination.export
	selection.mu.Lock()
	valid := selection.claimed && !selection.started && !selection.released && selection.request.Offer == offer
	if valid {
		selection.started = true
	}
	capability := selection.capability
	budget := selection.budget
	selection.mu.Unlock()
	if !valid || capability == nil {
		return nil, errors.New("invalid local export selection")
	}
	if !budget.take(offer.Size) {
		return nil, errors.New("private staging capacity unavailable")
	}
	success := false
	defer func() {
		if !success {
			budget.release(offer.Size)
		}
	}()
	// One private subdirectory per transfer. Do not publish the peer filename in
	// the staging root or force provider output through path/hard-link semantics.
	if err := os.MkdirAll(destination.dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.Lstat(destination.dir)
	if err != nil {
		return nil, err
	}
	if !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("private staging root unavailable")
	}
	dir, err := os.MkdirTemp(destination.dir, ".mnet-export-*")
	if err != nil {
		return nil, err
	}
	storage, err := beginPathReceive(dir, offer.Name)
	if err != nil {
		os.Remove(dir)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	watcher := make(chan struct{})
	go func() {
		defer close(watcher)
		select {
		case <-closed:
			cancel()
		case <-reset:
			cancel()
		case <-ctx.Done():
		}
	}()
	success = true
	return &stagedExportTransaction{budget: budget, stage: storage.(*pathReceiveTransaction), dir: dir, offer: offer, capability: capability, ctx: ctx, cancel: cancel, watcher: watcher, allowed: allowed}, nil
}
func (t *stagedExportTransaction) Write(p []byte) (int, error) { return t.stage.Write(p) }
func (t *stagedExportTransaction) Finalize() error             { return t.stage.Finalize() }
func (t *stagedExportTransaction) current() bool {
	if t.ctx.Err() != nil {
		return false
	}
	if !t.allowed() {
		t.cancel()
		return false
	}
	return t.ctx.Err() == nil
}
func (t *stagedExportTransaction) Publish() error {
	if t.used || !t.stage.finalized {
		return os.ErrInvalid
	}
	t.used = true
	if !t.current() {
		return context.Canceled
	}
	// Bound cooperative adapters before the sender's existing30second ack wait.
	// Native timeout UX and already-entered noncooperative calls remain separate.
	ctx, cancel := context.WithTimeout(t.ctx, 20*time.Second)
	defer cancel()
	current := func() bool { return ctx.Err() == nil && t.current() }
	reader, err := os.Open(t.stage.temp)
	if err != nil {
		return err
	}
	defer reader.Close()
	output, err := t.capability.Open(ctx, t.offer)
	// An adapter returning partial ownership on error still owes cleanup.
	if output != nil {
		defer output.Abort()
	}
	if err != nil {
		return err
	}
	if output == nil {
		return errors.New("export output unavailable")
	}
	if !current() {
		return context.Canceled
	}
	remaining := t.offer.Size
	buffer := make([]byte, fileChunk)
	for remaining > 0 {
		if !current() {
			return context.Canceled
		}
		count := int64(len(buffer))
		if remaining < count {
			count = remaining
		}
		n, err := io.ReadFull(reader, buffer[:count])
		if err != nil {
			return err
		}
		if !current() {
			return context.Canceled
		}
		written, err := output.Write(ctx, buffer[:n])
		if err != nil {
			return err
		}
		if written != n {
			return io.ErrShortWrite
		}
		remaining -= int64(n)
	}
	if !current() {
		return context.Canceled
	}
	if err := output.Finish(ctx); err != nil {
		return err
	}
	if !current() {
		return context.Canceled
	}
	return nil
}
func (t *stagedExportTransaction) Abort() {
	t.abortOnce.Do(func() {
		t.cancel()
		<-t.watcher
		t.stage.Abort()
		_ = os.Remove(t.dir)
		t.budget.release(t.offer.Size)
	})
}
