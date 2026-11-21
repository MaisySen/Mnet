//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
	"fmt"
)

// Stage executor remains unimplemented. Driver source fixes order and verdict
// semantics; it is not a backdoor to generic netlink or TUN. No retries.
type ConformanceStage interface {
	RunStage(context.Context, string) error
	StopClientAndJoin(context.Context) error
	CleanupAndVerify(context.Context) (baselineHash, finalHash string, err error)
}

var noTunStages = []string{"prove-namespace-userns-lifetime", "capture-full-baseline", "create-dummy-exclusive", "ack-alias-mtu", "disable-owned-ipv6-readback", "address32-exclusive", "link-up-exact", "route32-exclusive", "full-owned-readback"}

type StageFailure struct {
	Stage, Class string
	Cause        error
}

func (e *StageFailure) Error() string {
	return fmt.Sprintf("stage %s failed (%s): %v", e.Stage, e.Class, e.Cause)
}
func (e *StageFailure) Unwrap() error { return e.Cause }

// Even failed setup stops client activity before the separate cleanup reserve.
// Cleanup failure never becomes pass; no process-exit-only absence oracle.
func RunNoTunDriver(ctx, cleanupCtx context.Context, ns Namespace, s ConformanceStage) (Verdict, error) {
	v := Verdict{Version: 1, Namespace: ns, Sequence: 1, Result: "fail"}
	for _, stage := range noTunStages {
		if e := ctx.Err(); e != nil {
			v.Stage = stage
			v.FailureClass = "runner"
			return finishNoTun(cleanupCtx, s, v, e)
		}
		if e := s.RunStage(ctx, stage); e != nil {
			v.Stage = stage
			v.FailureClass = "runner"
			var f *StageFailure
			if errors.As(e, &f) {
				v.FailureClass = f.Class
			}
			return finishNoTun(cleanupCtx, s, v, e)
		}
	}
	v.Stage = "final"
	v.Result = "pass"
	return finishNoTun(cleanupCtx, s, v, nil)
}
func finishNoTun(ctx context.Context, s ConformanceStage, v Verdict, prior error) (Verdict, error) {
	if e := s.StopClientAndJoin(ctx); e != nil {
		v.Result = "fail"
		v.FailureClass = "cleanup"
		return v, errors.Join(prior, e)
	}
	a, b, e := s.CleanupAndVerify(ctx)
	v.BaselineHash = a
	v.FinalHash = b
	v.CleanupConfirmed = e == nil && a != "" && a == b
	if !v.CleanupConfirmed {
		v.Result = "fail"
		v.FailureClass = "cleanup"
		return v, errors.Join(prior, e, errors.New("full cleanup not confirmed"))
	}
	return v, prior
}
