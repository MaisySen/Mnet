#!/usr/bin/env python3
"""Compile the real compatibility helper against API-checking JVM fakes.
Not an Android emulator, device test, SDK verifier or full app lifecycle test.
"""
from pathlib import Path
import subprocess
import tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
APP = ROOT / 'android/app/src/main'
JAVA = APP / 'java/au/fintex/mnet'
A = '{http://schemas.android.com/apk/res/android}'
manifest = ET.parse(APP / 'AndroidManifest.xml').getroot()
permission = 'au.fintex.mnet.permission.PRIVATE_STATUS'
assert any(p.get(A + 'name') == permission and p.get(A + 'protectionLevel') == 'signature'
           for p in manifest.findall('permission')), 'private status permission must be signature-only'
assert any(p.get(A + 'name') == permission for p in manifest.findall('uses-permission'))
for filename in ('MainActivity.java', 'HostingActivity.java'):
    text = (JAVA / filename).read_text()
    assert 'VpnPlatform.registerPrivateReceiver(this,' in text, filename
    assert 'registerReceiver(' not in text.replace('unregisterReceiver(', ''), filename
for filename in ('MnetVpnService.java', 'HostSessionService.java'):
    text = (JAVA / filename).read_text()
    assert 'VpnPlatform.notification(this,' in text, filename
    assert 'new Notification.Builder' not in text and 'new NotificationChannel' not in text, filename

access = ET.parse(APP / 'res/xml/mnet_accessibility.xml').getroot()
assert access.get(A + 'canPerformGestures') == 'true'
assert access.get(A + 'canRetrieveWindowContent') == 'false', 'no window retrieval needed for gestures'
assert access.get(A + 'accessibilityEventTypes') is None, 'no events consumed by gesture-only service'
assert access.get(A + 'isAccessibilityTool') != 'true', 'not a disability-support tool'
service = (JAVA / 'MnetAccessibilityService.java').read_text()
assert 'getRootInActiveWindow' not in service and 'getWindows(' not in service

files = {
'android/os/Build.java': '''package android.os; public final class Build {
 public static final class VERSION { public static int SDK_INT; }
}''',
'android/os/Handler.java': 'package android.os; public class Handler {}',
'android/content/Intent.java': 'package android.content; public class Intent {}',
'android/content/IntentFilter.java': 'package android.content; public class IntentFilter {}',
'android/content/BroadcastReceiver.java': 'package android.content; public class BroadcastReceiver {}',
'android/content/Context.java': '''package android.content;
import android.os.*; import android.app.*;
public class Context {
 public static final int RECEIVER_NOT_EXPORTED = 4;
 public int oldCalls, newCalls; public String permission; public int flags;
 public final NotificationManager manager = new NotificationManager();
 public String getPackageName() { return "au.fintex.mnet"; }
 public <T> T getSystemService(Class<T> kind) { return kind.cast(manager); }
 public Intent registerReceiver(BroadcastReceiver r, IntentFilter f, String p, Handler h) {
  if (Build.VERSION.SDK_INT >= 33) throw new AssertionError("modern receiver missing flags");
  oldCalls++; permission=p; return null;
 }
 public Intent registerReceiver(BroadcastReceiver r, IntentFilter f, String p, Handler h, int g) {
  if (Build.VERSION.SDK_INT < 33) throw new AssertionError("modern receiver on legacy API");
  newCalls++; permission=p; flags=g; return null;
 }
}''',
'android/app/NotificationChannel.java': '''package android.app; import android.os.Build;
public class NotificationChannel {
 public NotificationChannel(String id, String name, int priority) {
  if (Build.VERSION.SDK_INT < 26) throw new AssertionError("channel on legacy API");
 }
}''',
'android/app/NotificationManager.java': '''package android.app; import android.os.Build;
public class NotificationManager {
 public static final int IMPORTANCE_LOW=2; public int creates; private NotificationChannel channel;
 public NotificationChannel getNotificationChannel(String id) {
  if (Build.VERSION.SDK_INT < 26) throw new AssertionError("channel lookup on legacy API");
  return channel;
 }
 public void createNotificationChannel(NotificationChannel c) { channel=c; creates++; }
}''',
'android/app/Notification.java': '''package android.app; import android.content.Context; import android.os.Build;
public class Notification {
 public static final int PRIORITY_LOW=-1;
 public static class Builder {
  public boolean modern; public int priority;
  public Builder(Context c) { if (Build.VERSION.SDK_INT >= 26) throw new AssertionError("missing channel"); }
  public Builder(Context c, String id) {
   if (Build.VERSION.SDK_INT < 26) throw new AssertionError("channel builder on legacy API"); modern=true;
  }
  public Builder setPriority(int p) { priority=p; return this; }
 }
}''',
'au/fintex/mnet/VpnPlatformTest.java': '''package au.fintex.mnet;
import android.app.*; import android.content.*; import android.os.Build;
public class VpnPlatformTest {
 public static void main(String[] args) {
  for (int sdk : new int[]{24,25,26,28,29,32,33,34,35,36}) {
   Build.VERSION.SDK_INT=sdk; Context c=new Context();
   Notification.Builder b=VpnPlatform.notification(c,"mesh","Mnet");
   VpnPlatform.notification(c,"mesh","Mnet");
   if (b.modern != (sdk>=26) || c.manager.creates != (sdk>=26 ? 1 : 0)) throw new AssertionError("channel policy");
   if (sdk<26 && b.priority != Notification.PRIORITY_LOW) throw new AssertionError("legacy priority");
   VpnPlatform.registerPrivateReceiver(c,new BroadcastReceiver(),new IntentFilter());
   if (!"au.fintex.mnet.permission.PRIVATE_STATUS".equals(c.permission)) throw new AssertionError("receiver permission");
   if (c.oldCalls!=(sdk<33?1:0) || c.newCalls!=(sdk>=33?1:0)) throw new AssertionError("receiver branch");
   if (sdk>=33 && c.flags!=Context.RECEIVER_NOT_EXPORTED) throw new AssertionError("export flag");
  }
  System.out.println("PASS Android API24-36 branch matrix, channel reuse, receiver signature permission/modern non-export flag; JVM fakes only");
 }
}'''
}
with tempfile.TemporaryDirectory(prefix='mnet-android-api-') as directory:
    temp = Path(directory)
    sources = []
    for name, text in files.items():
        path = temp / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        sources.append(str(path))
    subprocess.run(['javac', '-d', str(temp / 'classes'), *sources, str(JAVA / 'VpnPlatform.java')], check=True)
    subprocess.run(['java', '-cp', str(temp / 'classes'), 'au.fintex.mnet.VpnPlatformTest'], check=True)
print('PASS Android compatibility call-site and manifest source checks; not device/runtime proof')
