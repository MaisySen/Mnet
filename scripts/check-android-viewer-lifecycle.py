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
