//go:build mnet_enrollment_development

package osownership

import "errors"

var errOriginalOffloadHistory = errors.New("original-offload-history-missing-or-mismatched")

// Produced only at the actual original TUNSETOFFLOAD(0) return site, before
// the created-link observation or any supplier/ledger exists. No mark-success
// helper, caller boolean, TUNGETIFF alias or feature bitmap can create history.
// Package-internal construction still relies on review, not physical sealing.
type originalOffloadHistory struct {
	attempt   *originalAttempt
	custody   *fileCustody
	namespace Namespace
	name      string
	request   uint32
	argument  uint32
}

func (h *originalOffloadHistory) matches(a *originalAttempt, fd *fileCustody, ns Namespace, name string) bool {
	return h != nil && a != nil && fd != nil && h.attempt == a && h.custody == fd && a.custody == fd && a.offloadHistory == h && !a.closeAttempted && a.createErr == nil && h.namespace == ns && sameNamespace(ns, ns) && h.name == name && h.request == 0x400454d0 && h.argument == 0 && a.createRequest == 0x9001
}
