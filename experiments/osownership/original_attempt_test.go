//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type countOriginalCloser struct {
	calls  int
	err    error
	order  *[]string
	closed chan struct{}
	before func()
}

func (c *countOriginalCloser) Close() error {
	if c.before != nil {
		c.before()
	}
	c.calls++
	if c.closed != nil {
		close(c.closed)
	}
	if c.order != nil {
		*c.order = append(*c.order, "close")
	}
	return c.err
}

type fakeOriginalAbsence struct {
	order *[]string
	calls int
	fail  bool
	wrong bool
}

func (f *fakeOriginalAbsence) observeOriginalAbsence(_ context.Context, a *originalAttempt) (*originalAbsenceEvidence, error) {
	f.calls++
	if f.order != nil {
		*f.order = append(*f.order, "absence")
	}
	if f.fail {
		return nil, ErrConflict
	}
	if f.wrong {
		return &originalAbsenceEvidence{attempt: &originalAttempt{}, custody: a.custody}, nil
	}
	return &originalAbsenceEvidence{attempt: a, custody: a.custody}, nil
}
func TestOwnerCloseOwedRegardlessObservationAndNoHandle(t *testing.T) {
	closeFailure := errors.New("fake original close failed")
	for _, mode := range []string{"no-observer", "absence-failed", "absence-wrong", "absence-clean", "close-failed-clean-absence", "canceled-no-handle", "snapshot-failed", "rollback-foreign"} {
		t.Run(mode, func(t *testing.T) {
			c, source := operation1AdapterRig(t)
			var order []string
			closer := &countOriginalCloser{order: &order}
			if mode == "close-failed-clean-absence" {
				closer.err = closeFailure
			}
			c.attempt = &originalAttempt{owner: c, custody: &fileCustody{closer: closer}, stage: "ioctl-attempted", createErr: ErrConflict}
			// Optional Handle cannot be required for the close obligation.
			c.tx.handle = nil
			if mode == "snapshot-failed" {
				source.diagnoseErr = ErrConflict
				if e := c.withOwner(context.Background(), func(ctx context.Context) error { _, e := c.adapter.Snapshot(ctx); return e }); e == nil {
					t.Fatal("fake Snapshot should fail")
				}
			}
			if mode == "rollback-foreign" {
				h := *c.adapter.session.canonical
				h.Link.Index += 100
				c.tx.handle = &h
			}
			observer := &fakeOriginalAbsence{order: &order, fail: mode == "absence-failed", wrong: mode == "absence-wrong"}
			if mode != "no-observer" {
				c.absence = observer
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled-no-handle" {
				cancel()
			}
			e := c.stop(ctx)
			if mode == "snapshot-failed" && source.diagnoseCalls != 1 {
				t.Fatal("Stop attempted failed Snapshot again")
			}
			if closer.calls != 1 || len(order) == 0 || order[0] != "close" || !c.attempt.closeAttempted || c.tx.Ready || !c.tx.stopStarted {
				t.Fatal("owed close skipped/order", e, closer.calls, order)
			}
			clean := mode == "absence-clean" || mode == "snapshot-failed" || mode == "rollback-foreign"
			if clean {
				if e != nil || c.tx.PendingCleanup {
					t.Fatal("clean reconcile", e)
				}
			} else if e == nil || !c.tx.PendingCleanup {
				t.Fatal("false clean", e)
			}
			if mode == "close-failed-clean-absence" && (!errors.Is(e, closeFailure) || !c.attempt.absenceConfirmed) {
				t.Fatal("absence erased failed close", e)
			}
			_ = c.stop(context.Background())
			if closer.calls != 1 {
				t.Fatal("original close retried")
			}
		})
	}
}
func TestOriginalAttemptPublicationBeforeFallibleStep(t *testing.T) {
	for _, mode := range []string{"post-open-check", "ioctl-ambiguous", "readback", "attach-handle", "session-bind"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := operation1AdapterRig(t)
			closer := &countOriginalCloser{}

			e := c.withOwner(context.Background(), func(context.Context) error {
				c.attempt = &originalAttempt{owner: c, stage: "unopened"}
				// Production opener uses this SAME seam immediately after wrapping.
				return c.attempt.afterOpen(&fileCustody{closer: closer}, func() error {
					if c.attempt.custody == nil || closer.calls != 0 {
						t.Fatal("fallible check before retain")
					}
					switch mode {
					case "post-open-check":
						return ErrConflict
					case "ioctl-ambiguous":
						c.attempt.stage = "ioctl-attempted"
						return ErrConflict
					case "readback":
						c.attempt.stage = "ioctl-returned"
						return ErrConflict
					case "attach-handle":
						return ErrConflict
					case "session-bind":
						h := *c.adapter.session.canonical
						c.attempt.handle = &h
						return ErrConflict
					}
					return nil
				})
			})
			if e == nil || c.attempt.custody == nil || closer.calls != 0 {
				t.Fatal("error lost custody")
			}
			_ = c.stop(context.Background())
			_ = c.stop(context.Background())
			if closer.calls != 1 || !c.tx.PendingCleanup {
				t.Fatal("close/absence", closer.calls)
			}
		})
	}
}
func TestStopDuringApplyWaitsThenClosesOriginalOnce(t *testing.T) {
	c, source := operation1AdapterRig(t)
	source.entered = make(chan struct{})
	source.release = make(chan struct{})
	var order []string
	closer := &countOriginalCloser{order: &order, closed: make(chan struct{})}
	source.after = func() { order = append(order, "effect-complete") }
	c.attempt = &originalAttempt{owner: c, custody: &fileCustody{closer: closer}}
	h := *c.adapter.session.canonical
	var wg sync.WaitGroup
	wg.Add(2)
	applied := make(chan error, 1)
	stopped := make(chan error, 1)
	arrived := make(chan struct{})
	go func() {
		defer wg.Done()
		applied <- c.withOwner(context.Background(), func(ctx context.Context) error { return c.adapter.Apply(ctx, h, source.p.Operations[1]) })
	}()
	<-source.entered
	go func() { defer wg.Done(); close(arrived); stopped <- c.stop(context.Background()) }()
	<-arrived
	timer := time.NewTimer(30 * time.Millisecond)
	select {
	case <-closer.closed:
		timer.Stop()
		close(source.release)
		wg.Wait()
		t.Fatal("close overlapped Apply")
	case <-timer.C:
	}
	// No count read until both goroutines join. Channel only for negative oracle.
	close(source.release)
	wg.Wait()
	if e := <-applied; e != nil {
		t.Fatal(e)
	}
	if e := <-stopped; !errors.Is(e, errAbsenceUnverified) {
		t.Fatal(e)
	}
	if len(order) != 2 || order[0] != "effect-complete" || order[1] != "close" {
		t.Fatal("wrong apply/close ordering", order)
	}
	if closer.calls != 1 || source.sends != 1 || c.tx.Ready || !c.tx.PendingCleanup {
		t.Fatal("stop missed original")
	}
}

func TestCanceledStartStillClosesRetainedOriginal(t *testing.T) {
	c, _ := operation1AdapterRig(t)
	closer := &countOriginalCloser{}
	c.attempt = &originalAttempt{owner: c, custody: &fileCustody{closer: closer}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := c.start(ctx); e == nil || closer.calls != 1 || !c.tx.stopStarted || !c.tx.PendingCleanup {
		t.Fatal("early canceled Start skipped original", e, closer.calls)
	}
}
func TestFileCustodyCloserIsOnceAndSticky(t *testing.T) {
	failure := errors.New("fake close error")
	closer := &countOriginalCloser{err: failure}
	custody := &fileCustody{closer: closer}
	for n := 0; n < 3; n++ {
		if e := custody.Close(); !errors.Is(e, failure) {
			t.Fatal("close error lost", e)
		}
	}
	if closer.calls != 1 || !custody.closed {
		t.Fatal("close count", closer.calls)
	}
}

func TestOriginalCloseInvalidatesBeforeCloserEvenOnError(t *testing.T) {
	l, _, _ := firstStepRig(t)
	failure := errors.New("fake close failure")
	closer := &countOriginalCloser{err: failure, before: func() {
		state := l.snapshot()
		if !state.closed || !state.uncertain {
			t.Fatal("close effect before invalidation")
		}
	}}
	custody := &fileCustody{closer: closer, lineage: l}
	if e := custody.Close(); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if e := custody.Close(); !errors.Is(e, failure) || closer.calls != 1 {
		t.Fatal("reclose/lost error", e)
	}
}
