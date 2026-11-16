#!/usr/bin/env python3
"""Projection callback helper JVM fakes plus service wiring source checks.
Does not run Android, capture pixels, grant consent or test a real Go host.
"""
from pathlib import Path
import subprocess
import tempfile
ROOT = Path(__file__).resolve().parents[1]
JAVA = ROOT / 'android/app/src/main/java/au/fintex/mnet'
service = (JAVA / 'HostSessionService.java').read_text()
assert service.index('new ProjectionGuard(') < service.index('projection.createVirtualDisplay(')
assert 'START_STICKY' not in service, 'capture consent must not be replayed'
assert service.index('if (running) return') < service.index('startCapture(resultCode, data)')
assert 'projectionGuard.close()' in service and 'display.release()' in service and 'reader.close()' in service
assert 'latestJpeg = null;' in service and 'if (!running || latestSeq == servedSeq)' in service
assert 'if (!running) return;' in service and 'if (!running) return false;' in service
files = {
'android/os/Handler.java': 'package android.os; public class Handler {}',
'android/media/projection/MediaProjection.java': '''package android.media.projection;
import android.os.Handler;
public class MediaProjection {
 public static abstract class Callback { public void onStop() {} }
 public volatile Callback callback, oldCallback; public int registers, unregisters, stops;
 public void registerCallback(Callback c, Handler h) { registers++; callback=c; oldCallback=c; }
 public void unregisterCallback(Callback c) { unregisters++; if (callback==c) callback=null; }
 public void stop() { stops++; if (callback!=null) callback.onStop(); }
 public void externalStop() { if (oldCallback!=null) oldCallback.onStop(); }
 public void createDisplay() { if (callback==null) throw new IllegalStateException("missing callback"); }
}''',
'au/fintex/mnet/ProjectionGuardTest.java': '''package au.fintex.mnet;
import android.media.projection.MediaProjection;
import android.os.Handler;
import java.util.concurrent.atomic.AtomicInteger;
public class ProjectionGuardTest {
 public static void main(String[] args) throws Exception {
  try { new ProjectionGuard(null,new Handler(),()->{}); throw new AssertionError("null accepted"); }
  catch (IllegalStateException expected) {}
  MediaProjection p=new MediaProjection(); AtomicInteger notices=new AtomicInteger();
  ProjectionGuard g=new ProjectionGuard(p,new Handler(),notices::incrementAndGet);
  p.createDisplay(); p.externalStop(); p.externalStop(); g.close();
  if (notices.get()!=1 || p.registers!=1 || p.unregisters!=1 || p.stops!=0) throw new AssertionError("external-stop cleanup");
  p=new MediaProjection(); notices.set(0); g=new ProjectionGuard(p,new Handler(),notices::incrementAndGet);
  g.close(); g.close(); p.externalStop();
  if (notices.get()!=0 || p.unregisters!=1 || p.stops!=1) throw new AssertionError("app-close idempotence");
  for (int i=0;i<100;i++) {
   final MediaProjection projection=new MediaProjection(); AtomicInteger calls=new AtomicInteger();
   final ProjectionGuard guard=new ProjectionGuard(projection,new Handler(),calls::incrementAndGet);
   Thread a=new Thread(guard::close), b=new Thread(projection::externalStop);
   a.start();b.start();a.join();b.join();
   if (projection.unregisters!=1 || projection.stops>1 || calls.get()>1 || calls.get()+projection.stops!=1) throw new AssertionError("stop race");
  }
  System.out.println("PASS projection callback registration, null refusal, app/system stop idempotence and100 races; JVM fakes only");
 }
}'''
}
with tempfile.TemporaryDirectory(prefix='mnet-projection-') as directory:
    temp = Path(directory)
    sources = []
    for name, text in files.items():
        path = temp / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        sources.append(str(path))
    subprocess.run(['javac', '-d', str(temp / 'classes'), *sources, str(JAVA / 'ProjectionGuard.java')], check=True)
    subprocess.run(['java', '-cp', str(temp / 'classes'), 'au.fintex.mnet.ProjectionGuardTest'], check=True)
print('PASS capture service callback-order/release/nonsticky/duplicate-start/frame-gate wiring source checks; not Android runtime proof')
