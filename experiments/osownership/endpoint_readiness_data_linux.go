//go:build linux && mnet_enrollment_development

package osownership

// Poll DATA, not custody, a completion receipt or a cached policy decision.
// The source caller must retain the original interval and revalidate before I/O.
// Linux UAPI POLLIN is bit0. Other readiness bits do not grant a TUN read.
type endpointPollResult struct{ TUN, Input, Output int16 }

func (p endpointPollResult) tunReadable() bool { return p.TUN&1 != 0 }

// One-use scheduling hint only. A forged hint still cannot pass original
// ownership, genuine completion, expiry or fresh per-I/O policy validation.
type endpointReadHint struct{ poll endpointPollResult }

func (h *endpointReadHint) replace(p endpointPollResult) { h.poll = p }
func (h *endpointReadHint) take() bool {
	yes := h.poll.tunReadable()
	h.poll = endpointPollResult{}
	return yes
}
