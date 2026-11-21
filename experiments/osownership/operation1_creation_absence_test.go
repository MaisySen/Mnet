//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"testing"
)

type adapterCreationFake struct {
	controller      *operation1Controller
	closer          *countOriginalCloser
	handle          Handle
	fail            string
	observedAttempt *originalAttempt
}

func (f *adapterCreationFake) createOriginal(_ context.Context, p Plan, a *originalAttempt) (Handle, error) {
	f.observedAttempt = a
	if a != f.controller.attempt || a.owner != f.controller || a.custody != nil {
		panic("attempt not published")
	}
	if f.fail == "open" {
		return Handle{}, ErrConflict
	}
	if e := a.afterOpen(&fileCustody{closer: f.closer}, func() error {
		if f.fail == "post-open" {
			return ErrConflict
		}
		return nil
	}); e != nil {
		return Handle{}, e
	}
	if f.fail == "no-handle" {
		return Handle{}, ErrConflict
	}
	h := f.handle
	a.handle = &h
	return h, nil
}
func (f *adapterCreationFake) bindCreatedOriginal(context.Context, *originalAttempt) (operation1DiagnosticSource, error) {
	if f.fail == "bind" {
		return nil, ErrConflict
	}
	return &operation1FakeSource{}, nil // Never exercised as diagnostic/packet source.
}
func TestAdapterCreateOwnsAttemptBeforeDependencyAndStopOwnsClose(t *testing.T) {
	for _, mode := range []string{"success", "open", "post-open", "no-handle", "bind"} {
		t.Run(mode, func(t *testing.T) {
			c, f := operation1AdapterRig(t)
			s := c.adapter.session
			h := *s.canonical
			s.canonical = nil
			s.plan = nil
			s.forwardStep = 0
			s.createAttempted = false
			closer := &countOriginalCloser{}
			creator := &adapterCreationFake{controller: c, closer: closer, handle: h, fail: mode}
			c.adapter.creation = creator
			c.adapter.source = nil
			var got Handle
			e := c.withOwner(context.Background(), func(ctx context.Context) error { var e error; got, e = c.adapter.Create(ctx, f.p); return e })
			if c.attempt == nil || creator.observedAttempt != c.attempt {
				t.Fatal("lost attempt")
			}
			if mode == "success" {
				if e != nil || got != h || s.canonical == nil || s.forwardStep != 1 || c.adapter.source == nil {
					t.Fatal("create join", e)
				}
			} else if e == nil {
				t.Fatal("failure accepted")
			}
			_ = c.stop(context.Background())
			_ = c.stop(context.Background())
			want := 1
			if mode == "open" {
				want = 0
			}
			if closer.calls != want || c.tx.Ready {
				t.Fatal("wrong original close", closer.calls, want)
			}
			if mode != "open" && !c.tx.PendingCleanup {
				t.Fatal("close mistaken for absence")
			}
		})
	}
}

type rawAbsenceFake struct {
	baseline *CapturedBaseline
	raw      RawInventory
	sys      SysctlSnapshot
	reuse    *originalAbsenceEvidence
	proof    *originalAbsenceEvidence
}

func (f *rawAbsenceFake) observeOriginalAbsence(_ context.Context, a *originalAttempt) (*originalAbsenceEvidence, error) {
	if f.reuse != nil {
		return f.reuse, nil
	}
	p, e := parseOriginalAbsence(a, a.absenceTicket, f.baseline, f.raw, f.sys)
	f.proof = p
	return p, e
}
func TestScopedAbsenceExactEqualityAndTicketReplay(t *testing.T) {
	for _, mode := range []string{"same", "raw-drift", "sys-drift", "namespace-drift", "failed-close"} {
		t.Run(mode, func(t *testing.T) {
			c, f := operation1AdapterRig(t)
			baseline := c.adapter.session.baseline
			closer := &countOriginalCloser{}
			if mode == "failed-close" {
				closer.err = ErrConflict
			}
			c.attempt = &originalAttempt{owner: c, plan: f.p, custody: &fileCustody{closer: closer}}
			raw := cloneRawExact(baseline.raw)
			sys := cloneSysExact(SysctlSnapshot(baseline.sys))
			switch mode {
			case "raw-drift":
				raw.Links[0].MTU++
			case "sys-drift":
				sys["unreviewed"] = "1"
			case "namespace-drift":
				c.attempt.plan.Binding.Namespace.Inode++
			}
			observer := &rawAbsenceFake{baseline: baseline, raw: raw, sys: sys}
			c.absence = observer
			e := c.stop(context.Background())
			if mode == "same" {
				if e != nil || c.tx.PendingCleanup || !c.attempt.absenceConfirmed {
					t.Fatal("equal refusal", e)
				}
				old := observer.proof
				if old == nil || old.ticket.active || !old.ticket.used || c.attempt.absenceTicket != nil {
					t.Fatal("ticket escaped scope")
				}
				observer.reuse = old
				if e = c.stop(context.Background()); e == nil || !c.tx.PendingCleanup {
					t.Fatal("replayed ticket")
				}
			} else if e == nil || !c.tx.PendingCleanup {
				t.Fatal("mismatch/close falsely clean", e)
			}
			if closer.calls != 1 {
				t.Fatal("close retried")
			}
		})
	}
}
