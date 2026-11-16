#!/usr/bin/env python3
"""Source wiring checks only. ViewerGateTest owns the JVM state tests."""
from pathlib import Path
text = (Path(__file__).resolve().parents[1] / 'android/app/src/main/java/au/fintex/mnet/ViewerActivity.java').read_text()
for required in ('ViewerGate<SessionViewer>', 'viewers.begin()', 'viewers.publish(attempt, connected)',
                 'closeViewer(connected)', 'viewers.active(attempt)', 'viewers.end(attempt)',
                 'viewers.close()', 'reconnects.removeCallbacks(retryTask)',
                 'reconnects.postDelayed(retryTask, retryMs)', 'retryPending'):
    assert required in text, required
assert 'removeCallbacksAndMessages' not in text, 'late publication/cleanup must remain queued'
assert 'node.connectSession(peerIp, this)' not in text, 'callbacks need attempt identity'
assert 'viewer = node.connectSession' not in text, 'worker cannot directly publish viewer'
print('PASS Android viewer lifecycle wiring source checks; no Activity/runtime execution')

host = (Path(__file__).resolve().parents[1] / 'android/app/src/main/java/au/fintex/mnet/HostSessionService.java').read_text()
activity = (Path(__file__).resolve().parents[1] / 'android/app/src/main/java/au/fintex/mnet/HostingActivity.java').read_text()
for required in ('CONSENT.begin(this)', 'CONSENT.await(request, 60, TimeUnit.SECONDS)', 'CONSENT.cancel(this)', 'EXTRA_REQUEST, request.id'):
    assert required in host, required
assert 'pendingLatch' not in host and 'pendingAnswer' not in host
for required in ('EXTRA_REQUEST', 'consentPending(requestId)', 'answerSession(requestId, true)', 'answerSession(requestId, false)'):
    assert required in activity, required
print('PASS Android exact-request consent wiring source checks; no dialog/runtime execution')
