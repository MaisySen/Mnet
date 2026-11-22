//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"testing"
)

type preparedMemoryKernel struct {
	*fakeKernel
	completionCalls int
	refuse          bool
}

func (k *preparedMemoryKernel) complete(ctx context.Context, t *Transaction) (*completionReceipt, error) {
	k.completionCalls++
	if k.refuse {
		return nil, ErrConflict
	}
	return k.fakeKernel.complete(ctx, t)
}
func TestPreparedSetupRequiresMandatoryCompletion(t *testing.T) {
	for _, kind := range []string{"refused", "success", "stopped", "expired", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			tx, k, clock := transactionRig(t)
			wrapper := &preparedMemoryKernel{fakeKernel: k, refuse: kind == "refused"}
			tx.k = wrapper
			p, err := tx.prepareSetup(context.Background())
			if err != nil || !p.current(tx) || tx.Ready || !tx.PendingCleanup || wrapper.completionCalls != 0 {
				t.Fatal("preparation minted readiness or lost custody", err)
			}
			switch kind {
			case "stopped":
				if err := tx.Stop(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "expired":
				clock.s.Monotonic = tx.deadline.Mono
			case "foreign":
				other, _, _ := transactionRig(t)
				if err := other.completePrepared(context.Background(), p); err == nil || wrapper.completionCalls != 0 {
					t.Fatal("foreign token consumed")
				}
				return
			}
			err = tx.completePrepared(context.Background(), p)
			if kind == "success" {
				if err != nil || !tx.Ready || wrapper.completionCalls != 1 {
					t.Fatal("mandatory test-only completion not consumed", err)
				}
			} else if err == nil || tx.Ready {
				t.Fatal("incomplete/stopped/expired preparation became ready")
			}
			expected := 1
			if kind == "stopped" || kind == "expired" {
				expected = 0
			}
			if wrapper.completionCalls != expected {
				t.Fatal("unexpected completion attempt")
			}
			if err := tx.completePrepared(context.Background(), p); err == nil || wrapper.completionCalls != expected {
				t.Fatal("replayed preparation")
			}
		})
	}
}
