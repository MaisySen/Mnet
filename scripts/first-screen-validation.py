#!/usr/bin/env python3
"""Product first-screen deadline with actual host approval and built home."""
import os,pathlib,subprocess,tempfile,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-first-screen.'));print(out,flush=True)
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
try:
 displays=[]
 for role in ('host','viewer'):
  with (out/(role+'-display')).open('w') as fd,(out/(role+'-x.log')).open('w') as log:xs.append(subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log))
  displays.append(':'+poll(lambda:(out/(role+'-display')).read_text().strip(),role+' display'))
 env.update({'DISPLAY':displays[0],'NO_AT_BRIDGE':'1','DBUS_SESSION_BUS_ADDRESS':'unix:path=/nonexistent-mnet-fixture','MNET_NATIVE_VIEWER_TEST':'1','MNET_VIEWER_HOME':'1','MNET_ATTENDED_ARTIFACTS':str(out),'MNET_VIEWER_EXECUTABLE':str(out/'mnet'),'MNET_VIEWER_DISPLAY':displays[1],'MNET_VIEWER_CASE':'first-screen-stall'})
 env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 with (out/'test.log').open('w') as log:test=subprocess.Popen([str(out/'host.test'),'-test.run=^TestAttendedX11BuiltViewOnlyClient$','-test.v','-test.timeout=25s'],env=env,stdout=log,stderr=log)
 home=poll(lambda:window(displays[1],'^Mnet: three tools'),'home');action(displays[1],'windowfocus',home,'key','Tab','Tab','Return')
 chooser=poll(lambda:window(displays[1],'^Mnet Remote Desktop: choose \\[ready\\]'),'chooser');action(displays[1],'windowfocus',chooser,'mousemove','--window',chooser,'120','120','click','1','mousemove','--window',chooser,'470','510','click','1')
 host=poll(lambda:window(displays[0],'^Mnet: allow viewing'),'host approval');action(displays[0],'windowfocus',host,'key','Tab','Return')
 started=time.monotonic();viewer=poll(lambda:window(displays[1],'^Mnet view only:.*Connected'),'admitted no-screen state')
 subprocess.run(['import','-display',displays[1],'-window',viewer,str(out/'waiting-first-screen.png')],check=True)
 image=Image.open(out/'waiting-first-screen.png').convert('RGB')
 assert image.getpixel((220,312))==(28,42,59),'central first-screen panel absent'
 assert image.getpixel((880,15))==(155,52,62),'local End absent while loading'
 assert image.getpixel((330,30))==(21,25,35),'loading paint contaminated header background'
 home=poll(lambda:window(displays[1],'^Mnet: three tools'),'first-screen failure returns home',17)
 elapsed=time.monotonic()-started;assert 14<=elapsed<18,elapsed
 subprocess.run(['import','-display',displays[1],'-window',home,str(out/'first-screen-failed-home.png')],check=True)
 assert not window(displays[1],'^Mnet view only:'),'retired viewer survived'
 poll(lambda:not window(displays[0],'^Mnet: desktop sharing'),'host sharing closed')
 action(displays[1],'windowfocus',home,'key','Escape');assert test.wait(timeout=3)==0,(out/'test.log').read_text()
 print((out/'test.log').read_text(),flush=True)
 print('PASS actual host-approved silent capture ends at15second first-screen bound, old viewer/host close, fresh home failure state; no input owner/reconnect',flush=True)
finally:
 if test is not None and test.poll() is None:test.kill();test.wait()
 for x in xs:x.terminate();x.wait(timeout=3)
 print('Artifacts retained: '+str(out),flush=True)
