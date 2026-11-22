//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"time"
)

// Concrete fixed entry. It intentionally does not reuse the4500ms diagnostic
// verdict mode. Parent gate/custody/Pdeathsig and one held resident TID remain.
func endpointResidentBootstrapSource(ctx context.Context) (ret error) {
	if !kernelExecutionReleased {
		return ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil || os.Getppid() != parseSourceParentPID() {
		return ErrConflict
	}
	for _, fd := range []int{0, 3, 4, 5} {
		unix.CloseOnExec(fd)
	}
	var gate [1]byte
	if _, e := io.ReadFull(os.Stdin, gate[:]); e != nil || gate[0] != 1 {
		return ErrConflict
	}
	if os.Getppid() != parseSourceParentPID() {
		return ErrConflict
	}
	if e := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); e != nil {
		return e
	}
	_ = os.Stdin.Close()
	_ = unix.Close(3) // diagnostic verdict lane not used here
	in, out := os.NewFile(4, "endpoint-parent-read"), os.NewFile(5, "endpoint-parent-write")
	wire, e := newEndpointPipeWireSource(in, out, 1)
	if e != nil {
		if in != nil {
			in.Close()
		}
		if out != nil {
			out.Close()
		}
		return e
	}
	defer wire.closeSource()
	parent, e := parseSourceParentNamespace()
	if e != nil {
		return e
	}
	self, e := os.Open("/proc/thread-self/ns/net")
	if e != nil {
		return e
	}
	defer self.Close()
	return withResidentNamespace(int(self.Fd()), parent, func(ns *namespaceCustody) error {
		initial, cancel := context.WithTimeout(ctx, 5*time.Second)
		var first endpointFrame
		for {
			f, ok, e := wire.readSource()
			if e != nil {
				cancel()
				return e
			}
			if ok {
				first = f
				break
			}
			if e = wire.waitSource(initial, false); e != nil {
				cancel()
				return e
			}
		}
		cancel()
		if first.kind != endpointStart || first.sequence != 1 {
			return errEndpointFrame
		}
		start, e := decodeEndpointStart(first.payload)
		if e != nil {
			return e
		}
		end := time.UnixMilli(start.Prepared.SelfUntilMS)
		if other := time.UnixMilli(start.Prepared.PeerUntilMS); other.Before(end) {
			end = other
		}
		operation, stop := context.WithDeadline(ctx, end)
		defer stop()
		authority, e := newResidentEndpointAdmissionSource(operation, ns, wire, start)
		if e != nil {
			return e
		}
		var baseline *CapturedBaseline
		initialPiece, finishInitial, e := authority.beginForwardPieceSource(operation)
		if e != nil {
			return e
		}
		initialErr := func() error {
			raw, e := ReadStableInventory(initialPiece, residentNetlink{ns: ns}, ns.checkCurrent)
			if e != nil {
				return e
			}
			sys, e := readPolicySysctlsSource(initialPiece, ns, raw)
			if e != nil {
				return e
			}
			baseline, e = NewBaselineCandidate(parent, ns.identity, raw, sys)
			return e
		}()
		if e = finishInitial(initialErr); e != nil {
			return e
		}
		id := adapterIdentity{Namespace: ns.identity, TargetID: fmt.Sprint(ns.identity.Device, ":", ns.identity.Inode), SessionID: fmt.Sprint(start.Prepared.PID, ":", start.Prepared.StartTicks), AttemptID: "endpoint-v1-original"}
		stats := endpointStatsBody{Reason: endpointReasonCustody}
		var owned *operation1Controller
		runErr := withInheritedEndpointResidentSource(operation, ns, baseline, id, authority, func(c *operation1Controller) error {
			owned = c
			prep, e := c.prepareEndpoint(operation)
			if e != nil {
				return e
			}
			var r *originalEndpointReactor
			var body endpointPreparedBody
			e = c.withOwner(operation, func(lease context.Context) error {
				source, ok := c.adapter.source.(*operation1ScopedDiagnostic)
				if !ok {
					return afPhase
				}
				observation, e := source.collectPreparedEndpointSource(lease, prep)
				if e != nil {
					return e
				}
				r, e = newOriginalEndpointReactor(lease, c, observation)
				if e != nil {
					return e
				}
				body, e = observation.controlBodySource(lease)
				return e
			})
			if e != nil {
				return e
			}
			if body != start.Prepared {
				return ErrConflict
			}
			authority.prepared = &body
			if e = r.frames.consumeStart(first); e != nil {
				return e
			}
			payload, e := body.encode()
			if e != nil {
				return e
			}
			if e = r.frames.enqueue(endpointPrepared, payload); e != nil {
				return e
			}
			e = func() (ret error) {
				handshake, done := context.WithTimeout(operation, endpointHandshakeBudget)
				defer func() {
					if ret != nil {
						if operation.Err() != nil {
							stats.Reason = endpointReasonLifetime
						} else if errors.Is(handshake.Err(), context.DeadlineExceeded) {
							stats.Reason = endpointReasonTimeout
						}
					}
					done()
				}()
				for wire.sent == 0 {
					progress, e := flushEndpointFramesSource(r.frames, wire)
					if e != nil {
						done()
						return e
					}
					if !progress {
						if e = wire.waitSource(handshake, true); e != nil {
							done()
							return e
						}
					}
				}
				for !authority.started {
					if e = authority.checkMembershipSource(handshake, ns, c.tx.plan.Binding); e != nil {
						done()
						return e
					}
					f, ok, e := wire.readSource()
					if e != nil {
						stats.Reason = endpointReasonEOF
						done()
						return e
					}
					if !ok {
						if e = wire.waitSource(handshake, false); e != nil {
							done()
							return e
						}
						continue
					}
					if f.kind == endpointStop {
						stats.Reason = endpointReasonRequested
						done()
						return errEndpointStopped
					}
					expected := endpointAdmission
					if authority.admitted != nil {
						expected = endpointFlowStart
					}
					if f.kind != expected {
						stats.Reason = endpointReasonProtocol
						done()
						return errEndpointFrame
					}
					if _, e = r.frames.receive(f); e != nil {
						done()
						return e
					}
					if e = handshake.Err(); e != nil {
						done()
						return e
					}
					if f.kind == endpointAdmission {
						e = authority.acceptSource(f)
					} else {
						e = authority.acceptFlowSource(f)
					}
					if e != nil {
						done()
						return e
					}
				}
				if e = handshake.Err(); e != nil {
					done()
					return e
				}
				done()
				return nil
			}()
			if e != nil {
				return e
			}
			// Mandatory genuine local completion, not a native daemon pointer and not
			// a remote capability promoted to proof. The source assessment still leaves
			// all named semantic rows unsupported, so no genuine completion is available.
			if e = c.completePreparedEndpoint(operation, prep); e != nil {
				stats.Reason = endpointReasonPolicyIncomplete
				return e
			}
			receipt := c.tx.currentCompletion
			if receipt == nil {
				return afPhase
			}
			evidence, ok := receipt.evidence.(*sourceRuntimeEvidence)
			if !ok {
				return afPhase
			}
			if e = r.armSource(operation, evidence, receipt, authority); e != nil {
				return e
			}
			inbox := &endpointBridgeInbox{}
			loop := &endpointResidentLoop{}
			lane := &endpointOriginalLane{reactor: r, inbox: inbox, ctx: operation, cleanup: context.WithoutCancel(operation)}
			defer func() { stats.Counts = loop.counts }()
			for {
				if operation.Err() != nil {
					stats.Reason = endpointReasonLifetime
					return loop.end(lane, operation.Err())
				}
				f, input, e := wire.readSource()
				if e != nil {
					stats.Reason = endpointReasonEOF
					return loop.end(lane, e)
				}
				if input {
					if e = inbox.push(f); e != nil {
						return loop.end(lane, e)
					}
				}
				before := loop.counts
				if e = loop.turnInbox(lane, inbox); e != nil {
					if errors.Is(e, errEndpointStopped) {
						stats.Reason = endpointReasonRequested
					}
					return e
				}
				wrote, e := flushEndpointFramesSource(r.frames, wire)
				if e != nil {
					return loop.end(lane, e)
				}
				idle := !input && !wrote && loop.counts.Read == before.Read && loop.counts.PolicyDrops == before.PolicyDrops && loop.counts.KernelDrops == before.KernelDrops && loop.counts.QueueDrops == before.QueueDrops
				// Every turn polls the original TUN. Busy turns use timeout0 so
				// continuous inherited input cannot starve the opposite direction.
				// Idle turns wait<=50ms under unchanged original custody. A timeout
				// grants no TUN read and therefore triggers no per-I/O netlink dump.
				// All actual packet I/O still performs fresh before/after policy reads.
				ready, e := r.waitReadySource(operation, wire, idle)
				if e != nil {
					return loop.end(lane, e)
				}
				lane.readHint.replace(ready)
			}
		})
		if owned != nil {
			state := owned.state()
			stats.CleanupVerified = state.Stopped && !state.Ready && !state.PendingCleanup && owned.tx.fdCloseError == nil && owned.endpointCleanup != nil && owned.endpointCleanup.finished && owned.endpointCleanup.err == nil
		}
		stats.Reason = endpointFinalReasonData(runErr, stats.Reason, stats.CleanupVerified)
		// Best-effort bounded final counters. No payload/error text/native data.
		// Missing acknowledgement is never reported as verified remote cleanup.
		final, finish := context.WithTimeout(context.WithoutCancel(ctx), endpointFinalStatsBudget)
		defer finish()
		if wire.ended == nil {
			wire.pending = nil
			if p, e := stats.encode(); e == nil {
				if e = wire.queueSource(endpointStopped, p); e == nil {
					for wire.pending != nil {
						ok, e := wire.flushSource()
						if e != nil {
							break
						}
						if !ok {
							if e = wire.waitSource(final, true); e != nil {
								break
							}
						}
					}
				}
			}
		}
		return runErr
	})
}
func flushEndpointFramesSource(s *endpointFrameState, w *endpointPipeWire) (bool, error) {
	if w.pending == nil {
		raw, e := s.nextWrite()
		if e == errEndpointBackpressure {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		if e = w.queueRawSource(raw); e != nil {
			_ = s.finishWrite(0, e)
			return false, e
		}
	}
	n := len(w.pending)
	progress, e := w.flushSource()
	if e != nil {
		_ = s.finishWrite(0, e)
		return false, e
	}
	if progress {
		if e = s.finishWrite(n, nil); e != nil {
			return false, e
		}
	}
	return progress, nil
}
