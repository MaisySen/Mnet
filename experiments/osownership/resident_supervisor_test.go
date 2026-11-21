//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type fakeResident struct {
	ns        Namespace
	r         io.Reader
	calls     []string
	reapErr   error
	closePipe func()
}

func (f *fakeResident) Namespace() Namespace     { return f.ns }
func (f *fakeResident) VerdictReader() io.Reader { return f.r }
func (f *fakeResident) Reap(context.Context) error {
	f.calls = append(f.calls, "reap")
	return f.reapErr
}
func (f *fakeResident) AbortAndReap(context.Context) error {
	f.calls = append(f.calls, "abort-reap")
	if f.closePipe != nil {
		f.closePipe()
	}
	return nil
}
func (f *fakeResident) CloseCustody() error { f.calls = append(f.calls, "close"); return nil }

type fakeSpawn struct{ p *fakeResident }

func (f fakeSpawn) Spawn(context.Context) (ResidentProcess, error) { return f.p, nil }
func validVerdict() Verdict {
	return Verdict{Version: 1, Stage: "final", Namespace: Namespace{Device: 7, Inode: 9, Isolated: true}, Sequence: 1, Result: "pass", BaselineHash: strings.Repeat("a", 64), FinalHash: strings.Repeat("a", 64), CleanupConfirmed: true}
}
func TestSupervisorVerdictJoinBeforeCustodyClose(t *testing.T) {
	v := validVerdict()
	p := &fakeResident{ns: v.Namespace, r: bytes.NewReader(mustJSON(v))}
	if _, e := SuperviseResident(context.Background(), fakeSpawn{p}); e != nil || !reflect.DeepEqual(p.calls, []string{"reap", "close"}) {
		t.Fatal(e, p.calls)
	}
}
func TestSupervisorVerdictFailuresAbortReapBeforeClose(t *testing.T) {
	for _, mode := range []string{"size", "duplicate", "trailing", "wrong-ns", "unknown-field", "no-cleanup", "decode-failure", "non-hex", "reap-failed"} {
		v := validVerdict()
		b := mustJSON(v)
		switch mode {
		case "size":
			b = make([]byte, maxVerdictBytes+1)
		case "duplicate":
			b = append(b, b...)
		case "trailing":
			b = append(b, 'x')
		case "wrong-ns":
			v.Namespace.Inode++
			b = mustJSON(v)
		case "unknown-field":
			b = append([]byte(`{"unknown":0,`), b[1:]...)
		case "no-cleanup":
			v.CleanupConfirmed = false
			b = mustJSON(v)
		case "decode-failure":
			v.Result = "fail"
			v.FailureClass = "decoder"
			b = mustJSON(v)
		case "non-hex":
			v.BaselineHash = strings.Repeat("z", 64)
			v.FinalHash = v.BaselineHash
			b = mustJSON(v)
		}
		p := &fakeResident{ns: validVerdict().Namespace, r: bytes.NewReader(b)}
		if mode == "reap-failed" {
			p.reapErr = errors.New("unreaped")
		}
		if _, e := SuperviseResident(context.Background(), fakeSpawn{p}); e == nil || p.calls[len(p.calls)-1] != "close" {
			t.Fatal("bad verdict accepted", mode, e)
		}
		if p.calls[len(p.calls)-2] != "abort-reap" {
			t.Fatal("did not reap before closing namespace", mode, p.calls)
		}
	}
}
func TestSupervisorCancellationClosesPipeAndJoins(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	p := &fakeResident{ns: validVerdict().Namespace, r: r, closePipe: func() { r.Close() }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := SuperviseResident(ctx, fakeSpawn{p}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(p.calls, []string{"abort-reap", "close"}) {
		t.Fatal(p.calls)
	}
}

type stuckResident struct{ *fakeResident }

func (f stuckResident) AbortAndReap(context.Context) error {
	f.calls = append(f.calls, "abort-unreaped")
	return errors.New("still alive")
}

type stuckSpawn struct{ p stuckResident }

func (s stuckSpawn) Spawn(context.Context) (ResidentProcess, error) { return s.p, nil }
func TestUnreapedChildRetainsCustody(t *testing.T) {
	p := &fakeResident{ns: validVerdict().Namespace, r: bytes.NewReader([]byte("invalid"))}
	_, e := SuperviseResident(context.Background(), stuckSpawn{stuckResident{p}})
	var pending *PendingResidentError
	if !errors.As(e, &pending) || !reflect.DeepEqual(p.calls, []string{"abort-unreaped"}) {
		t.Fatal("unreaped child lost custody", e, p.calls)
	}
}
