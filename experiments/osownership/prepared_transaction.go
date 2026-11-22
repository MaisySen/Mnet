//go:build mnet_enrollment_development

package osownership

import "context"

// PREPARED means only six setup operations/readbacks completed. It is private,
// one-shot, and never a completion receipt, Ready flag or packet-use permit.
// Original Transaction/controller still own cleanup and original deadlines.
type preparedTransaction struct {
	owner  *Transaction
	handle Handle
	used   bool
}

func (p *preparedTransaction) current(t *Transaction) bool {
	return p != nil && t != nil && p.owner == t && t.prepared == p && !p.used && !t.Ready && !t.stopStarted && !t.fdCloseAttempted && t.PendingCleanup && t.handle != nil && sameSessionHandle(p.handle, *t.handle)
}
func (c *operation1Controller) prepareEndpoint(ctx context.Context) (*preparedTransaction, error) {
	var p *preparedTransaction
	err := c.withOwner(ctx, func(ctx context.Context) error { var e error; p, e = c.tx.prepareSetup(ctx); return e })
	if err != nil {
		return nil, err
	}
	return p, nil
}
func (c *operation1Controller) completePreparedEndpoint(ctx context.Context, p *preparedTransaction) error {
	return c.withOwner(ctx, func(ctx context.Context) error { return c.tx.completePrepared(ctx, p) })
}
