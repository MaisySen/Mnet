package session

import (
	"context"
	"errors"
	"sync"
)

// Private callback shape matching the compile-tested native binding proposal.
// No mobile API, production provider or native entry point implements it yet.
// Write is synchronous, full-write-or-error; bytes must not be retained/mutated.
// Cancel may race a method and must be harmless after successful Finish.
type nativeFileOutput interface {
	Open() error
	Write([]byte) error
	Finish() error
	Cancel()
}

var errNativeFileExport = errors.New("native file export unavailable")

type nativeFileExport struct {
	output                      nativeFileOutput
	context                     context.Context
	mu                          sync.Mutex
	started, released, finished bool
	stop                        chan struct{}
	watched                     chan struct{}
	cancelOnce                  sync.Once
	releaseOnce                 sync.Once
}

func newNativeFileExport(output nativeFileOutput) *nativeFileExport {
	return &nativeFileExport{output: output, stop: make(chan struct{}), watched: make(chan struct{})}
}
func (x *nativeFileExport) cancelOutput() {
	x.cancelOnce.Do(func() {
		x.mu.Lock()
		x.released = true
		x.mu.Unlock()
		if x.output != nil {
			x.output.Cancel()
		}
	})
}
func (x *nativeFileExport) Release() {
	x.releaseOnce.Do(func() {
		x.mu.Lock()
		x.released = true
		started := x.started
		x.mu.Unlock()
		close(x.stop)
		x.cancelOutput()
		if started {
			<-x.watched
		}
	})
}
func (x *nativeFileExport) current(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.started && !x.released && !x.finished && x.context.Err() == nil
}
func (x *nativeFileExport) Open(ctx context.Context, _ FileOffer) (fileExportOutput, error) {
	x.mu.Lock()
	if x.output == nil || x.started || x.released || ctx.Err() != nil {
		x.mu.Unlock()
		return nil, errNativeFileExport
	}
	x.started = true
	x.context = ctx
	x.mu.Unlock()
	go func() {
		defer close(x.watched)
		select {
		case <-ctx.Done():
			x.cancelOutput()
		case <-x.stop:
		}
	}()
	// Even a failed/late Open returns owned cleanup to the receiver.
	if !x.current(ctx) {
		return x, errNativeFileExport
	}
	if err := x.output.Open(); err != nil {
		return x, errNativeFileExport
	}
	if !x.current(ctx) {
		return x, errNativeFileExport
	}
	return x, nil
}
func (x *nativeFileExport) Write(ctx context.Context, bytes []byte) (int, error) {
	if !x.current(ctx) {
		return 0, errNativeFileExport
	}
	if err := x.output.Write(bytes); err != nil {
		return 0, errNativeFileExport
	}
	if !x.current(ctx) {
		return 0, errNativeFileExport
	}
	return len(bytes), nil
}
func (x *nativeFileExport) Finish(ctx context.Context) error {
	if !x.current(ctx) {
		return errNativeFileExport
	}
	if err := x.output.Finish(); err != nil {
		return errNativeFileExport
	}
	if !x.current(ctx) {
		return errNativeFileExport
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.released || ctx.Err() != nil {
		return errNativeFileExport
	}
	x.finished = true
	return nil
}
func (x *nativeFileExport) Abort() { x.Release() }
