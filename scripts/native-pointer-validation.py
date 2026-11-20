#!/usr/bin/env python3
"""Built native pointer viewer and actual attended host on disposable X11."""
import os,pathlib,shutil,subprocess,tempfile,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for dep in ('go','Xvfb','xdotool','import','zenity'):
 if not shutil.which(dep):raise SystemExit('missing dependency: '+dep)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-native-pointer.'));print(out,flush=True)
for name,pkg,tests in [('mnet','./cmd/mnet',False),('mnetd','./cmd/mnetd',False),('host.test','./cmd/mnetd',True)]:
 subprocess.run(['go',*(['test','-race','-c'] if tests else ['build','-race']),'-o',str(out/name),pkg],check=True)
xs=[];test=None;focus_app=None
size=os.environ.get('MNET_VIEWER_DISPLAY_SIZE','1024x768x24')
if size not in ('1024x768x24','2048x1536x24'):raise SystemExit('unsupported disposable size')
def poll(fn,label,seconds=5):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  v=fn()
  if v:return v
  if test is not None and test.poll() is not None:raise AssertionError((out/'test.log').read_text())
  time.sleep(.02)
 raise AssertionError('deadline: '+label)
def window(display,pattern):
 result=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env|{'DISPLAY':display},capture_output=True,text=True)
 return result.stdout.strip().splitlines()[-1] if result.returncode==0 and result.stdout.strip() else None
def action(display,*args):subprocess.run(['xdotool',*args],env=env|{'DISPLAY':display},check=True)
def capture(display,wid,name):
 subprocess.run(['import','-display',display,'-window',wid,str(out/(name+'.png'))],check=True)
 if display==displays[1]:
  image=Image.open(out/(name+'.png')).convert('RGB');assert image.getpixel((image.width-80,15))==(155,52,62),'local End pixels absent'
def focused_pixels(wid,name,x):
 capture(displays[1],wid,name)
 return Image.open(out/(name+'.png')).convert('RGB').getpixel((x,11))==(255,255,255)
try:
 displays=[]
 for role in ('host','viewer'):
  with open(out/(role+'-display'),'w') as fd,open(out/(role+'-x.log'),'w') as log:
   xs.append(subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0',size if role=='host' else '1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log))
  number=poll(lambda:(out/(role+'-display')).read_text().strip(),role+' display');displays.append(':'+number)
 env=os.environ|{'DISPLAY':displays[0],'NO_AT_BRIDGE':'1','DBUS_SESSION_BUS_ADDRESS':'unix:path=/nonexistent-mnet-fixture','MNET_NATIVE_VIEWER_TEST':'1','MNET_ATTENDED_X11_TEST':'1','MNET_ATTENDED_ARTIFACTS':str(out),'MNET_VIEWER_EXECUTABLE':str(out/'mnet'),'MNET_ATTENDED_TEST_EXECUTABLE':str(out/'mnetd'),'MNET_VIEWER_DISPLAY':displays[1],'MNET_VIEWER_CASE':'pointer'}
 env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 with open(out/'test.log','w') as log:test=subprocess.Popen([str(out/'host.test'),'-test.run=^TestAttendedX11BuiltPointerClient$','-test.v','-test.timeout=30s'],env=env,stdout=log,stderr=log)
 host=poll(lambda:window(displays[0],'^Mnet: allow viewing'),'host view approval');capture(displays[0],host,'view-approval')
 viewer=poll(lambda:window(displays[1],'^Mnet pointer only:'),'viewer waiting');capture(displays[1],viewer,'waiting')
 action(displays[0],'windowfocus',host,'key','Tab','Return')
 def title():return subprocess.check_output(['xdotool','getwindowname',viewer],env=env|{'DISPLAY':displays[1]},text=True)
 poll(lambda:'[pointer available]' in title(),'pointer available')
 def actual_screen():
  capture(displays[1],viewer,'allowed')
  image=Image.open(out/'allowed.png').convert('RGB')
  return sum(n for n,c in (image.getcolors(960*672) or []) if c[2]>c[0]+30 and c[1]>c[0]+10)>10000
 poll(actual_screen,'actual host desktop pixels before pointer approval')
 capture(displays[1],viewer,'viewing-before-control')
 action(displays[1],'windowfocus',viewer,'mousemove','--window',viewer,'120','580','click','1','key','a')
 time.sleep(.1)
 if (out/'native-clicks').exists():raise AssertionError('view admission enabled native pointer')
 action(displays[1],'windowfocus',viewer,'key','Tab','Tab')
 poll(lambda:focused_pixels(viewer,'keyboard-ask-focus',547),'actual Ask focus ring pixels')
 action(displays[1],'key','Return')
 host=poll(lambda:window(displays[0],'^Mnet: allow pointer only'),'first control');capture(displays[0],host,'control-approval');action(displays[0],'windowfocus',host,'key','Tab','Return')
 poll(lambda:'[pointer allowed]' in title(),'host grants pointer');capture(displays[1],viewer,'allowed')
 # Bottom-left lies outside host dialogs and inside the native target window.
 # Viewer is960x672, frame fit is800x600 with left gutter80 at this aspect.
 def click():action(displays[1],'windowfocus',viewer,'mousemove','--window',viewer,'120','580','click','1')
 click();poll(lambda:(out/'native-clicks').exists() and (out/'native-clicks').read_text()=='1','first actual native click')
 action(displays[1],'key','a','click','4');poll(lambda:(out/'native-scroll').exists() and (out/'native-scroll').read_text()=='1','actual scroll before local release')
 action(displays[1],'key','Tab','Tab','space');poll(lambda:'[pointer declined]' in title(),'keyboard local release');capture(displays[1],viewer,'released');click();time.sleep(.1)
 # Fresh request, actual host decline, no native owner/input.
 action(displays[1],'key','F8');host=poll(lambda:window(displays[0],'^Mnet: allow pointer only'),'decline control');action(displays[0],'windowfocus',host,'key','Return')
 poll(lambda:'[pointer declined]' in title(),'host decline');click();time.sleep(.1)
 # Fresh allow then actual host Revoke, preserving viewing.
 action(displays[1],'key','F8');host=poll(lambda:window(displays[0],'^Mnet: allow pointer only'),'second allow');action(displays[0],'windowfocus',host,'key','Tab','Return')
 poll(lambda:'[pointer allowed]' in title(),'second grant');click();poll(lambda:(out/'native-clicks').read_text()=='2','second actual native click')
 focus_app=subprocess.Popen(['xmessage','-geometry','150x100+20+20','-buttons','OK:0','Local focus target, not remote input'],env=env|{'DISPLAY':displays[1]},stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 focus_window=poll(lambda:window(displays[1],'^xmessage$'),'local focus window')
 action(displays[1],'key','Tab','keydown','Return','windowfocus',focus_window)
 poll(lambda:'[pointer declined]' in title(),'focus loss releases pointer');capture(displays[1],viewer,'focus-released')
 action(displays[1],'keyup','Return','windowfocus',viewer,'keyup','Return');assert test.poll() is None,'focus loss retained pending End'
 focus_app.terminate();focus_app.wait();focus_app=None
 action(displays[1],'windowfocus',viewer,'key','F8');host=poll(lambda:window(displays[0],'^Mnet: allow pointer only'),'third allow');action(displays[0],'windowfocus',host,'key','Tab','Return')
 poll(lambda:'[pointer allowed]' in title(),'third grant')
 active=poll(lambda:window(displays[0],'^Mnet: desktop sharing'),'host active')
 # Extra-button hit geometry is checked by the existing attended fixture;
 # click the inspected button band, then require actual viewer revoke state.
 capture(displays[0],active,'host-active-before-revoke')
 image=Image.open(out/'host-active-before-revoke.png')
 width,height=image.size;px,py=width-75,round(height*.86)
 if not (width>500 and height>200 and 0<px<width and height*.78<py<height*.96):raise AssertionError('unrecognized revoke fixture geometry')
 action(displays[0],'windowraise',active,'mousemove','--sync','--window',active,str(px),str(py),'click','1')
 poll(lambda:'[pointer revoked]' in title(),'host revoke');capture(displays[1],viewer,'revoked');click();time.sleep(.1)
 action(displays[1],'key','Tab')
 poll(lambda:focused_pixels(viewer,'keyboard-end-focus',843),'actual End focus ring pixels')
 action(displays[1],'key','space');assert test.wait(timeout=5)==0,(out/'test.log').read_text()
 print((out/'test.log').read_text(),flush=True)
 print('PASS built pointer viewer: host-enforced pointer-only approval, two actual native clicks and scroll, keyboard Ask/Release/End, local release/focus loss, host decline/revoke, no keyboard/held buttons, joined End',flush=True)
finally:
 if focus_app is not None:focus_app.terminate();focus_app.wait()
 if test is not None and test.poll() is None:test.kill();test.wait()
 for p in xs:p.terminate();p.wait()
