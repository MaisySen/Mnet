//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeStages struct {
	calls      []string
	fail       string
	cleanupErr error
}

func (f *fakeStages) RunStage(_ context.Context, s string) error {
	f.calls = append(f.calls, s)
	if s == f.fail {
		return &StageFailure{Stage: s, Class: "decoder", Cause: errors.New("recorded decoder refusal")}
	}
	return nil
}
func (f *fakeStages) StopClientAndJoin(context.Context) error {
	f.calls = append(f.calls, "stop-client-join")
	return nil
}
func (f *fakeStages) CleanupAndVerify(context.Context) (string, string, error) {
	f.calls = append(f.calls, "cleanup-full-baseline")
	return strings.Repeat("a", 64), strings.Repeat("a", 64), f.cleanupErr
}
func TestNoTunDriverStopsTrafficBeforeCleanupEveryCut(t *testing.T) {
	for _, fail := range append([]string{""}, noTunStages...) {
		f := &fakeStages{fail: fail}
		v, e := RunNoTunDriver(context.Background(), context.Background(), validVerdict().Namespace, f)
		if (e == nil) != (fail == "") || !v.CleanupConfirmed {
			t.Fatal(fail, e, v)
		}
		if f.calls[len(f.calls)-2] != "stop-client-join" || f.calls[len(f.calls)-1] != "cleanup-full-baseline" {
			t.Fatal("cleanup before client joined")
		}
		if fail != "" && v.FailureClass != "decoder" {
			t.Fatal("decoder failure mislabeled", v)
		}
	}
}
func TestDriverCleanupFailureNeverPasses(t *testing.T) {
	f := &fakeStages{cleanupErr: errors.New("unknown")}
	v, e := RunNoTunDriver(context.Background(), context.Background(), validVerdict().Namespace, f)
	if e == nil || v.Result != "fail" || v.CleanupConfirmed {
		t.Fatal("unverified cleanup passed")
	}
}
