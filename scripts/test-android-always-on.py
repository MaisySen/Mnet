#!/usr/bin/env python3
"""Real API29 mode helper against version-checking JVM fakes, not Android runtime."""
from pathlib import Path
import subprocess, tempfile
root = Path(__file__).resolve().parents[1]
source = root / 'android/app/src/main/java/au/fintex/mnet'
files = {
'android/os/Build.java': 'package android.os; public class Build {public static class VERSION {public static int SDK_INT;}}',
'android/net/VpnService.java': '''package android.net; public class VpnService {
 public boolean enabled; public int reads;
 public boolean isAlwaysOn(){if(android.os.Build.VERSION.SDK_INT<29)throw new AssertionError("API29 on old SDK");reads++;return enabled;}
}''',
'au/fintex/mnet/AlwaysOnTest.java': '''package au.fintex.mnet;
public class AlwaysOnTest {public static void main(String[]args){
 for(int sdk:new int[]{24,25,26,28,29,33,34})for(boolean mode:new boolean[]{false,true}){
  android.os.Build.VERSION.SDK_INT=sdk;android.net.VpnService s=new android.net.VpnService();s.enabled=mode;
  if(VpnAlwaysOn.managed(s)!=(sdk>=29&&mode))throw new AssertionError("wrong policy");
  if(s.reads!=(sdk>=29?1:0))throw new AssertionError("unguarded API");
 }
 System.out.println("PASS 14 mode/API combinations; JVM fakes only");
}}'''}
with tempfile.TemporaryDirectory(prefix='mnet-always-on-') as tmp:
    out = Path(tmp)
    for name, text in files.items():
        p = out / name; p.parent.mkdir(parents=True, exist_ok=True); p.write_text(text)
    subprocess.run(['javac', '-d', tmp, str(source / 'VpnAlwaysOn.java'), *[str(out / f) for f in files]], check=True)
    subprocess.run(['java', '-cp', tmp, 'au.fintex.mnet.AlwaysOnTest'], check=True)
service = (source / 'MnetVpnService.java').read_text()
activity = (source / 'MainActivity.java').read_text()
assert 'if (VpnAlwaysOn.managed(this)) {' in service
assert 'if (!VpnAlwaysOn.managed(this)) {' in service
assert 'i.putExtra(EXTRA_ALWAYS_ON, VpnAlwaysOn.managed(this))' in service
assert 'toggle.setEnabled(!connected || !alwaysOn)' in activity
assert 'if (alwaysOn) return;' in activity
print('PASS source wiring checks only; no notification/UI/device proof')
