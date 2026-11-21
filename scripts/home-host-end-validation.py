#!/usr/bin/env python3
"""Actual host-ended viewing session returns a specific home outcome with owners retired."""
import os,pathlib,subprocess,tempfile,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-host-end.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'host.test'),'./cmd/mnetd'],check=True)
xs=[];test=None;env=os.environ.copy()
def poll(fn,label,seconds=5):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  value=fn()
  if value:return value
  if test is not None and test.poll() is not None:raise AssertionError((out/'test.log').read_text())
  time.sleep(.02)
 raise AssertionError('deadline: '+label)
def window(display,pattern):
 r=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env|{'DISPLAY':display},capture_output=True,text=True)
 return r.stdout.strip().splitlines()[-1] if r.returncode==0 and r.stdout.strip() else None
def action(display,*args):subprocess.run(['xdotool',*args],env=env|{'DISPLAY':display},check=True)
def title(display,wid):return subprocess.check_output(['xdotool','getwindowname',wid],env=env|{'DISPLAY':display},text=True)
try:
 displays=[]
 for role in ('host','viewer'):
  with (out/(role+'-display')).open('w') as fd,(out/(role+'-x.log')).open('w') as log:xs.append(subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log))
  displays.append(':'+poll(lambda:(out/(role+'-display')).read_text().strip(),role+' display'))
 env.update({'DISPLAY':displays[0],'NO_AT_BRIDGE':'1','DBUS_SESSION_BUS_ADDRESS':'unix:path=/nonexistent-mnet-fixture','MNET_NATIVE_VIEWER_TEST':'1','MNET_VIEWER_HOME':'1','MNET_ATTENDED_ARTIFACTS':str(out),'MNET_VIEWER_EXECUTABLE':str(out/'mnet'),'MNET_VIEWER_DISPLAY':displays[1],'MNET_VIEWER_CASE':'host-end'})
 env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 with (out/'test.log').open('w') as log:test=subprocess.Popen([str(out/'host.test'),'-test.run=^TestAttendedX11BuiltViewOnlyClient$','-test.v','-test.timeout=25s'],env=env,stdout=log,stderr=log)
 home=poll(lambda:window(displays[1],'^Mnet: three tools'),'home');action(displays[1],'windowfocus',home,'key','Tab','Tab','Return')
 chooser=poll(lambda:window(displays[1],'^Mnet Remote Desktop: choose \\[ready\\]'),'chooser');action(displays[1],'windowfocus',chooser,'mousemove','--window',chooser,'120','120','click','1','mousemove','--window',chooser,'470','510','click','1')
 host_approval=poll(lambda:window(displays[0],'^Mnet: allow viewing'),'host approval');action(displays[0],'windowfocus',host_approval,'key','Tab','Return')
 viewer=poll(lambda:window(displays[1],'^Mnet view only:'),'admitted viewer')
 # Real pixels must arrive before the host end: the central waiting-panel
 # pixel is replaced by actual captured content.
 def first_frame():
  subprocess.run(['import','-display',displays[1],'-window',viewer,str(out/'frame-check.png')],check=True,capture_output=True)
  return Image.open(out/'frame-check.png').convert('RGB').getpixel((220,312))!=(28,42,59)
 poll(first_frame,'actual first screen',10)
 active=poll(lambda:window(displays[0],'^Mnet: desktop sharing'),'host active')
 # The host ends the admitted session from its own control: an abnormal
 # mid-session end for the viewer, after real pixels arrived.
 action(displays[0],'windowfocus',active,'key','Return')
 home=poll(lambda:window(displays[1],'^Mnet: three tools'),'host-end home return',10)
 poll(lambda:'note9' in title(displays[1],home),'specific host-end notice')
 subprocess.run(['import','-display',displays[1],'-window',home,str(out/'home-after-host-end.png')],check=True)
 image=Image.open(out/'home-after-host-end.png').convert('RGB');assert image.size==(900,560)
 # The retired viewer is gone and the host's own sharing UI retired too; the
 # harness independently fails on any safety/capture cleanup error.
 assert not window(displays[1],'^Mnet view only:'),'retired viewer survived host end'
 poll(lambda:not window(displays[0],'^Mnet: desktop sharing'),'host sharing retired')
 action(displays[1],'windowfocus',home,'key','Escape');assert test.wait(timeout=5)==0,(out/'test.log').read_text()
 print((out/'test.log').read_text(),flush=True)
 print('PASS actual host-end after first screen: specific home outcome, viewer/host windows retired, host capture and safety cleanup joined; same-host fixture only',flush=True)
finally:
 if test is not None and test.poll() is None:test.kill();test.wait()
 for p in xs:p.terminate();p.wait()
 print('Artifacts retained: '+str(out),flush=True)
