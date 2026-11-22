//go:build linux && mnet_enrollment_development && mnet_os_process_launch

package osownership

import (
	"context"
	"errors"
	"fintex.au/mnet/internal/daemon"
	"net/netip"
	"os"
	"runtime"
	"time"
)

// Exclusive parent driver lane. Native session stays here, never in child argv,
// shared memory, an exported descriptor or a serialized authority constructor.
type endpointParentCounts struct{ ControlRead, ControlWritten, AuthorityRevokes, FramingErrors, Stops, MissingStats, DatagramsDropped, DatagramsToPeer, DatagramsToChild uint64 }
type endpointParentSource struct {
	child                    *residentChild
	wire                     *endpointPipeWire
	peer                     *daemon.DevelopmentEndpointPeer
	bridge                   *daemon.DevelopmentEndpointBridge
	policy                   *endpointPacketPolicy
	lifetime                 context.Context
	stopBridge               func() bool
	started                  bool
	binding                  Binding
	start                    endpointStartBody
	prepared                 *endpointPreparedBody
	admission                *endpointAdmissionBody
	counts                   endpointParentCounts
	lastStats                *endpointStatsBody
	stopResult               *endpointParentStopResult
	cancel                   context.CancelFunc
	stopNative, stopLifetime func() bool
}

func spawnEndpointFilesSource(ctx context.Context, files []*os.File) (ResidentProcess, error) {
	type answer struct {
		p ResidentProcess
		e error
	}
	ch := make(chan answer, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		p, e := spawnResidentPinnedModeFiles(ctx, endpointResidentMode, files)
		ch <- answer{p, e}
		if c, ok := p.(*residentChild); ok {
			<-c.wait
		} else {
			var pending *PendingResidentError
			if errors.As(e, &pending) {
				if c, ok := pending.Process.(*residentChild); ok {
					<-c.wait
				}
			}
		}
	}()
	a := <-ch
	return a.p, a.e
}
func newEndpointParentSource(ctx context.Context, peer *daemon.DevelopmentEndpointPeer, b Binding) (p *endpointParentSource, ret error) {
	if !kernelExecutionReleased {
		return nil, ErrExecutorUnreleased
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrConflict
	}
	if e := requireEndpointMembershipBinding(peer, b); e != nil {
		return nil, e
	}
	childIn, parentOut, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	defer childIn.Close()
	parentIn, childOut, e := os.Pipe()
	if e != nil {
		parentOut.Close()
		return nil, e
	}
	defer childOut.Close()
	wire, e := newEndpointPipeWireSource(parentIn, parentOut, 2)
	if e != nil {
		parentIn.Close()
		parentOut.Close()
		return nil, e
	}
	process, e := spawnEndpointFilesSource(ctx, []*os.File{childIn, childOut})
	if e != nil {
		wire.closeSource()
		return nil, e
	}
	child, ok := process.(*residentChild)
	if !ok {
		wire.closeSource()
		return nil, ErrConflict
	}
	// Parent copies of the child-only ends close before START/ADMISSION. These
	// duplicate inherited endpoints are never retained during the dataplane.
	if e = errors.Join(childIn.Close(), childOut.Close()); e != nil {
		failed := &endpointParentSource{child: child, wire: wire}
		return nil, errors.Join(e, failed.stopSource(ctx))
	}
	b.Namespace = child.ns
	p = &endpointParentSource{child: child, wire: wire, peer: peer, binding: b}
	lifetime, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.lifetime = lifetime
	p.stopNative = context.AfterFunc(peer.Context(), cancel)
	p.stopLifetime = context.AfterFunc(lifetime, func() { _ = wire.closeWriteSource() })
	owned := p
	defer func() {
		if ret != nil {
			ret = errors.Join(ret, owned.stopSource(ctx))
		}
	}()
	own, e := endpointKey(b.OwnKey)
	if e != nil {
		return nil, e
	}
	other, e := endpointKey(b.PeerKey)
	if e != nil {
		return nil, e
	}
	self, remote, e := peer.MembershipSnapshot()
	if e != nil {
		return nil, e
	}
	ip, e := endpointIPv4(self.IPv4)
	if e != nil {
		return nil, e
	}
	remoteIP, e := endpointIPv4(remote.IPv4)
	if e != nil {
		return nil, e
	}
	p.start = endpointStartBody{Prepared: endpointPreparedBody{PID: uint32(child.cmd.Process.Pid), StartTicks: child.startTicks, Namespace: child.ns, Generation: 1, MTU: MTU, Own: ip, Peer: remoteIP, OwnKey: own, PeerKey: other, SelfUntilMS: b.SelfExpiry.UnixMilli(), PeerUntilMS: b.PeerExpiry.UnixMilli()}, Name: b.Name, Alias: b.Alias}
	payload, e := p.start.encode()
	if e != nil {
		return nil, e
	}
	if e = wire.queueSource(endpointStart, payload); e != nil {
		return nil, e
	}
	if e = p.flushSource(ctx); e != nil {
		return nil, e
	}
	return p, nil
}
func (p *endpointParentSource) currentSource(ctx context.Context) (ret error) {
	if p != nil {
		defer func() {
			if ret != nil && p.wire != nil {
				endpointCount(&p.counts.AuthorityRevokes)
				ret = errors.Join(ret, p.wire.closeWriteSource())
			}
		}()
	}
	if p == nil || p.child == nil || p.wire == nil || p.lifetime == nil || p.lifetime.Err() != nil || ctx == nil || ctx.Err() != nil {
		return ErrConflict
	}
	if e := pidfdAlive(p.child.pidfd); e != nil {
		return e
	}
	if p.admission == nil {
		return requireEndpointMembershipBinding(p.peer, p.binding)
	}
	current, e := compareEndpointPreparedParentSource(ctx, p.child, p.peer, p.bridge, p.binding, *p.prepared)
	if e != nil || current != *p.admission {
		return errors.Join(e, ErrConflict)
	}
	return nil
}
func (p *endpointParentSource) flushSource(ctx context.Context) error {
	for p.wire.pending != nil {
		if e := p.currentSource(ctx); e != nil {
			return e
		}
		ok, e := p.wire.flushSource()
		if e != nil {
			return e
		}
		if ok {
			endpointCount(&p.counts.ControlWritten)
		}
		if !ok {
			if e = p.wire.waitSource(ctx, true); e != nil {
				return e
			}
		}
	}
	return p.currentSource(ctx)
}
func (p *endpointParentSource) preparedSource(ctx context.Context) (endpointPreparedBody, error) {
	var zero endpointPreparedBody
	if p.prepared != nil {
		return zero, ErrConflict
	}
	for {
		if e := p.currentSource(ctx); e != nil {
			return zero, e
		}
		f, ok, e := p.wire.readSource()
		if e != nil {
			return zero, e
		}
		if !ok {
			if e = p.wire.waitSource(ctx, false); e != nil {
				return zero, e
			}
			continue
		}
		if f.kind == endpointStopped || f.kind == endpointStats {
			if stats, e := decodeEndpointStats(f.payload); e == nil {
				p.lastStats = &stats
				endpointCount(&p.counts.ControlRead)
			}
			return zero, ownedAFPolicyIncomplete
		}
		if f.kind != endpointPrepared {
			return zero, errEndpointFrame
		}
		b, e := decodeEndpointPrepared(f.payload)
		if e != nil || b != p.start.Prepared {
			return zero, errEndpointFrame
		}
		endpointCount(&p.counts.ControlRead)
		p.prepared = &b
		return b, nil
	}
}

// Call only after explicit native publication/Refresh/BindProfiles in parent.
// No HTTP request, credential renewal or remote kernel proof is hidden here.
func (p *endpointParentSource) admitSource(ctx context.Context) (ret error) {
	defer func() {
		if ret != nil && p != nil && p.bridge != nil {
			ret = errors.Join(ret, p.wire.closeWriteSource(), p.bridge.Close())
		}
	}()
	if p == nil || p.prepared == nil || p.admission != nil {
		return ErrConflict
	}
	if p.bridge != nil || p.policy != nil || p.lifetime == nil || p.lifetime.Err() != nil {
		return ErrConflict
	}
	bridge, e := p.peer.ClaimPrivateEndpointBridge()
	if e != nil {
		return e
	}
	policy, e := newEndpointPacketPolicy(packetFramingReadback{Own: netip.AddrFrom4(p.prepared.Own), Peer: netip.AddrFrom4(p.prepared.Peer), LocalMTU: int(p.prepared.MTU), PeerMTU: MTU, TUN: true, NoPI: true, NoVNET: true, NoOffload: true})
	if e != nil {
		_ = bridge.Close()
		return e
	}
	p.bridge = bridge
	p.policy = policy
	// Old lifetime callback never reads the mutable bridge field. Replace it with
	// closures capturing this exact owner; racing old close is still fail-closed.
	p.stopLifetime()
	wire := p.wire
	p.stopLifetime = context.AfterFunc(p.lifetime, func() { _ = bridge.SignalWake(); _ = wire.closeWriteSource() })
	p.stopBridge = context.AfterFunc(bridge.Context(), func() { _ = bridge.SignalWake(); _ = wire.closeWriteSource() })
	b, e := compareEndpointPreparedParentSource(ctx, p.child, p.peer, p.bridge, p.binding, *p.prepared)
	if e != nil {
		return e
	}
	payload, e := b.encode()
	if e != nil {
		return e
	}
	// The sole concrete issuer writes only to this exact launched child's private
	// inherited pipe, while retaining its native owner, pidfd/start/ns custody.
	p.admission = &b
	if e = p.wire.queueSource(endpointAdmission, payload); e != nil {
		p.admission = nil
		return e
	}
	if e = p.flushSource(ctx); e != nil {
		p.wire.closeWriteSource()
		return e
	}
	// ADMISSION is now actually written. Only now can this never-running peer
	// start, on the exact already-announced port. No automatic recreate/retry.
	if e = bridge.StartPacketFlow(p.peer); e != nil {
		return e
	}
	flow, e := (endpointFlowBody{BridgeGeneration: b.BridgeGeneration, PairDigest: b.BridgePairDigest}).encode()
	if e != nil {
		return e
	}
	if e = p.wire.queueSource(endpointFlowStart, flow); e != nil {
		return e
	}
	if e = p.flushSource(ctx); e != nil {
		return e
	}
	p.started = true
	return nil
}

type endpointParentStopResult struct{ err error }

func (p *endpointParentSource) stopSource(ctx context.Context) (ret error) {
	if p == nil {
		return nil
	}
	if p.stopResult != nil {
		return p.stopResult.err
	}
	defer func() {
		if p.bridge != nil {
			ret = errors.Join(ret, p.bridge.Close())
			if p.stopResult != nil {
				p.stopResult.err = ret
			}
		}
	}()
	if ctx == nil {
		ctx = context.TODO()
	}
	endpointCount(&p.counts.Stops)
	if p.stopBridge != nil {
		p.stopBridge()
	}
	if p.bridge != nil {
		_ = p.bridge.SignalWake()
	}
	if p.stopNative != nil {
		p.stopNative()
	}
	if p.stopLifetime != nil {
		if !p.stopLifetime() && p.wire != nil {
			_ = p.wire.closeWriteSource()
		}
	}
	if p.cancel != nil {
		defer p.cancel()
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), endpointGraceBudget)
	defer cancel()
	var result error
	if p.wire != nil && p.wire.ended == nil {
		// Only an atomic, still-unwritten frame can be discarded. Ambiguous partial
		// transport writes already closed this wire and cannot enter this branch.
		p.wire.pending = nil
		if !p.wire.writeClosed.Load() {
			if e := p.wire.queueSource(endpointStop, endpointStopPayload(endpointReasonRequested)); e == nil {
				for p.wire.pending != nil && cleanup.Err() == nil {
					ok, e := p.wire.flushSource()
					if e != nil {
						break
					}
					if ok {
						endpointCount(&p.counts.ControlWritten)
					} else {
						if e = p.wire.waitSource(cleanup, true); e != nil {
							break
						}
					}
				}
			}
		}
		for p.wire.pending == nil && p.wire.ended == nil && p.lastStats == nil && cleanup.Err() == nil {
			f, ok, e := p.wire.readSource()
			if e != nil {
				break
			}
			if ok {
				if f.kind == endpointDatagram && p.admission != nil {
					endpointCount(&p.counts.DatagramsDropped)
					continue
				}
				if f.kind == endpointPrepared && p.prepared == nil {
					b, e := decodeEndpointPrepared(f.payload)
					if e == nil && b == p.start.Prepared {
						endpointCount(&p.counts.ControlRead)
						continue
					}
				}
				if f.kind != endpointStopped && f.kind != endpointStats {
					endpointCount(&p.counts.FramingErrors)
					break
				}
				stats, e := decodeEndpointStats(f.payload)
				if e != nil {
					endpointCount(&p.counts.FramingErrors)
					break
				}
				p.lastStats = &stats
				endpointCount(&p.counts.ControlRead)
				break
			}
			if e = p.wire.waitSource(cleanup, false); e != nil {
				break
			}
		}
	}
	if p.lastStats == nil {
		endpointCount(&p.counts.MissingStats)
		result = errors.Join(result, errEndpointCleanupUnverified)
	} else if !p.lastStats.CleanupVerified {
		result = errors.Join(result, errEndpointCleanupUnverified)
	}
	if p.wire != nil {
		result = errors.Join(result, p.wire.closeWriteSource())
	}
	if p.child != nil {
		// Fresh still-owned close/reap authority, never a membership extension.
		reapCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
		reap := p.child.Reap(reapCtx)
		finish()
		if reap != nil {
			abortCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
			abort := p.child.AbortAndReap(abortCtx)
			finish()
			if abort != nil {
				if p.wire != nil {
					result = errors.Join(result, p.wire.closeSource())
				}
				// Keep pidfd/ns custody available on this recoverable error. Never close
				// them and falsely report an un-reaped child as owned teardown complete.
				return errors.Join(result, reap, &PendingResidentError{Process: p.child, Cause: abort})
			}
			result = errors.Join(result, reap)
		}
		result = errors.Join(result, p.child.CloseCustody())
	}
	if p.wire != nil {
		result = errors.Join(result, p.wire.closeSource())
	}
	p.stopResult = &endpointParentStopResult{result}
	return result
}
