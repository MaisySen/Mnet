//go:build mnet_enrollment_development

package daemon

import (
	"errors"
	"fintex.au/mnet/internal/netmap"
	"testing"
)

func TestEndpointPublicationFailedWithdrawalRetainsCloseRetry(t *testing.T) {
	for _, kind := range []string{"failed-withdraw-close-ack", "failed-withdraw-close-unknown", "failed-positive-publish", "acknowledged-withdraw"} {
		t.Run(kind, func(t *testing.T) {
			state := endpointPublicationState{mayBePublished: true}
			var calls []bool
			unknown := errors.New("memory client unknown remote outcome")
			send := func(c *netmap.EndpointCapability) error {
				calls = append(calls, c != nil)
				if len(calls) == 1 && kind != "acknowledged-withdraw" {
					return unknown
				}
				if kind == "failed-withdraw-close-unknown" {
					return unknown
				}
				return nil
			}
			var c *netmap.EndpointCapability
			if kind == "failed-positive-publish" {
				value := netmap.EndpointCapability{Version: 1, Mode: netmap.EndpointModeV1, MTU: 1280}
				c = &value
				state.mayBePublished = false
			}
			err := state.publish(c, send)
			if kind == "acknowledged-withdraw" {
				if err != nil || state.mayBePublished {
					t.Fatal("acknowledged withdrawal retained claim")
				}
				if err = state.withdrawForClose(send); err != nil || len(calls) != 1 {
					t.Fatal("close repeated acknowledged withdrawal")
				}
				return
			}
			if err == nil || !state.mayBePublished || state.closeAttempted {
				t.Fatal("unknown request forgot potential remote publication")
			}
			err = state.withdrawForClose(send)
			if len(calls) != 2 || calls[1] || !state.closeAttempted {
				t.Fatal("close did not attempt exactly one actual withdrawal")
			}
			if kind == "failed-withdraw-close-unknown" {
				if err == nil || !state.mayBePublished {
					t.Fatal("unknown close outcome treated as acknowledgement")
				}
			} else if err != nil || state.mayBePublished {
				t.Fatal("acknowledged close outcome not consumed")
			}
			_ = state.withdrawForClose(send)
			if len(calls) != 2 {
				t.Fatal("repeated close created an unbounded retry")
			}
		})
	}
}
