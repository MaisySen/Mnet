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

assert 'static final AtomicBoolean autoAccept' not in host
assert 'intent.getBooleanExtra(EXTRA_AUTO_ACCEPT, false)' in host
assert 'autoAccept = false;' in host
assert 'HostSessionService.autoAccept' not in activity
assert 'capturePending && pendingAutoAccept' in activity
assert 'autoAccept.setChecked(false)' in activity
assert 'HostSessionService.EXTRA_AUTO_ACCEPT, autoAcceptThisHost' in activity
print('PASS host-run auto-accept default-false/reset and capture-request snapshot source checks; no consent/UI runtime proof')

assert 'CaptureEncoder.jpeg(' in host
assert host.index('Image img = null;') < host.index('try {', host.index('Image img = null;')) < host.index('img = r.acquireLatestImage();')
assert 'if (img != null) img.close();' in host
encoder = (Path(__file__).resolve().parents[1] / 'android/app/src/main/java/au/fintex/mnet/CaptureEncoder.java').read_text()
assert 'finally {' in encoder and 'pixels.duplicate()' in encoder and 'frame.recycle()' in encoder
print('PASS capture acquire/close and temporary bitmap release wiring source checks; not real ImageReader/memory-pressure proof')

assert 'ev.getActionMasked()' in text and 'taps.up(ev.getX(), ev.getY(), slop)' in text
assert 'taps.move(ev.getX(), ev.getY(), slop)' in text and 'ev.getPointerCount() != 1' in text
assert '"down".equals(action)' not in text
print('PASS viewer completed-tap/move/cancel/multitouch wiring source checks; no MotionEvent/device execution')

assert 'private volatile int frameW, frameH;' in text
assert 'viewer == null || frameW <= 0 || frameH <= 0' in text
assert 'frameW = frameH = 0;' in text and 'screen.setImageDrawable(null)' in text
assert 'if (frameW != bmp.getWidth() || frameH != bmp.getHeight()) taps.cancel();' in text
print('PASS frame-readiness/reset/resize-cancellation source wiring; no physical frame/input proof')

assert 'inputGeometryInvalid || frameW <= 0 || frameH <= 0' in host
assert 'captureDisplayW = devW;' in host and 'captureDisplayH = devH;' in host
assert 'captureRotation = wm.getDefaultDisplay().getRotation();' in host
assert 'inputGeometryInvalid = false;' in host and 'inputGeometryInvalid = true;' in host
assert 'dm.widthPixels != captureDisplayW || dm.heightPixels != captureDisplayH' in host
assert 'wm.getDefaultDisplay().getRotation() != captureRotation' in host
print('PASS host capture/display mismatch refusal/reset source wiring; no physical rotation/input proof')

assert 'if (!supportedPointer(evJson)) return;' in host
assert 'value >= limit || value != Math.floor(value)' in host
accessibility = (Path(__file__).resolve().parents[1] / 'android/app/src/main/java/au/fintex/mnet/MnetAccessibilityService.java').read_text()
assert 'case "down":' not in accessibility and 'case "up":' not in accessibility
assert 'Float.isNaN(x)' in accessibility and 'Float.isInfinite(scale)' in accessibility
print('PASS host pointer boundary/endpoint source checks; real accessibility dispatch not executed')

assert 'node.newSessionConnect()' in text
assert 'connecting.connectWithControl(peerIp, callbacks(attempt)' in text
assert text.count('pendingConnect.cancel()') == 2, 'failure and terminal teardown cancel their own pending attempt'
assert 'if(pendingConnect==connecting)pendingConnect=null;' in text
print('PASS per-attempt pending connect cancellation wiring; Go ownership tests and framework fakes are separate evidence')
