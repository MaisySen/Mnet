#!/usr/bin/env python3
"""View-only desktop acceptance on two owned disposable X displays."""
import subprocess,os,tempfile,pathlib,time,shutil
from PIL import Image,ImageChops
size=os.environ.get('MNET_VIEWER_DISPLAY_SIZE','1024x768x24')
if size not in ('1024x768x24','2048x1536x24'):raise SystemExit('unsupported owned display size')
for program in ['go','Xvfb','xdotool','import','xsetroot','xmessage']:
 if not shutil.which(program):raise SystemExit('missing native fixture dependency: '+program)
os.chdir(pathlib.Path(__file__).resolve().parent.parent);out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-native-viewer.'));print(out,flush=True)
subprocess.run(['go','build','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'host.test'),'./cmd/mnetd'],check=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'viewer.test'),'./cmd/mnet'],check=True)
xs=[];test=None
try:
 displays=[]
 for role in ['host','viewer']:
  with open(out/(role+'-display'),'w') as fd,open(out/(role+'-xvfb.log'),'w') as log:
   xs.append(subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0',os.environ.get('MNET_VIEWER_DISPLAY_SIZE','1024x768x24')],pass_fds=(fd.fileno(),),stdout=log,stderr=log))
  for _ in range(100):
   number=(out/(role+'-display')).read_text().strip()
   if number:break
   time.sleep(.02)
  displays.append(':'+number)
 env=os.environ|{'DISPLAY':displays[0],'NO_AT_BRIDGE':'1','DBUS_SESSION_BUS_ADDRESS':'unix:path=/nonexistent-mnet-fixture','MNET_NATIVE_VIEWER_TEST':'1','MNET_ATTENDED_ARTIFACTS':str(out),'MNET_VIEWER_EXECUTABLE':str(out/'mnet'),'MNET_VIEWER_DISPLAY':displays[1]};env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 ve=env|{'DISPLAY':displays[1]}
 subprocess.run(['xsetroot','-solid','#416fa8'],env=env,check=True)
 def window(env,pattern):
  p=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env,capture_output=True,text=True)
  return p.stdout.strip().splitlines()[-1] if p.returncode==0 and p.stdout.strip() else None
 # Pixel/resize/expose acceptance of the actual protocol viewer. Poll the
 # observed postcondition instead of guessing how long X event delivery takes.
 protocol=out/'protocol';protocol.mkdir()
 pe=ve|{'MNET_VIEWER_X11_TEST':'1','MNET_VIEWER_ARTIFACTS':str(protocol)}
 with open(protocol/'test.log','w') as log:
  test=subprocess.Popen([str(out/'viewer.test'),'-test.run','^TestDesktopViewX11ProtocolPixelsAndClose$','-test.v','-test.timeout=20s'],env=pe,stdout=log,stderr=log)
  until=time.monotonic()+4;wid=None
  while time.monotonic()<until:
   wid=window(ve,'^Mnet view only:.*Viewing.')
   if wid:break
   if test.poll() is not None:raise AssertionError((protocol/'test.log').read_text())
   time.sleep(.02)
  assert wid,'protocol image not shown'
  subprocess.run(['import','-window',wid,str(protocol/'live.png')],env=ve,check=True)
  subprocess.run(['xdotool','windowsize','--sync',wid,'700','520'],env=ve,check=True)
  until=time.monotonic()+3;fitted=False
  while time.monotonic()<until:
   subprocess.run(['import','-window',wid,str(protocol/'resized.png')],env=ve,check=True)
   image=Image.open(protocol/'resized.png').convert('RGB')
   a,b=image.getpixel((70,100)),image.getpixel((400,100))
   fitted=a[0]>150 and a[1]<50 and b[1]>150 and b[0]<50
   if fitted:break
   time.sleep(.02)
  assert fitted,'resized image landmarks do not fit expected geometry'
  subprocess.run(['xdotool','windowunmap',wid,'windowmap','--sync',wid],env=ve,check=True)
  until=time.monotonic()+3;exposed=False
  while time.monotonic()<until:
   subprocess.run(['import','-window',wid,str(protocol/'exposed.png')],env=ve,check=True)
   image=Image.open(protocol/'exposed.png').convert('RGB');a,b=image.getpixel((70,100)),image.getpixel((400,100))
   exposed=a[0]>150 and a[1]<50 and b[1]>150 and b[0]<50
   if exposed:break
   time.sleep(.02)
  assert exposed,'remapped window did not redraw actual image landmarks'
  subprocess.run(['xdotool','windowfocus',wid,'key','1'],env=ve,check=True)
  until=time.monotonic()+3;one=False
  while time.monotonic()<until:
   subprocess.run(['import','-window',wid,str(protocol/'one-to-one.png')],env=ve,check=True)
   image=Image.open(protocol/'one-to-one.png').convert('RGB');c=image.getpixel((400,100))
   one=c[0]>150 and c[1]<50
   if one:break
   time.sleep(.02)
  assert one,'local1 key did not display received pixels1:1'
  subprocess.run(['xdotool','key','End'],env=ve,check=True)
  until=time.monotonic()+3;panned=False
  while time.monotonic()<until:
   subprocess.run(['import','-window',wid,str(protocol/'panned.png')],env=ve,check=True)
   image=Image.open(protocol/'panned.png').convert('RGB');c=image.getpixel((400,100))
   panned=c[1]>150 and c[2]>150 and c[0]<50
   if panned:break
   time.sleep(.02)
  assert panned,'1:1 End pan did not reach bottom-right received image'
  subprocess.run(['xdotool','key','f'],env=ve,check=True)
  until=time.monotonic()+3;restored=False
  while time.monotonic()<until:
   subprocess.run(['import','-window',wid,str(protocol/'restored-fit.png')],env=ve,check=True)
   image=Image.open(protocol/'restored-fit.png').convert('RGB');a,b=image.getpixel((70,100)),image.getpixel((400,100))
   restored=a[0]>150 and a[1]<50 and b[1]>150 and b[0]<50
   if restored:break
   time.sleep(.02)
  assert restored,'localF did not restore fit after pan'
  subprocess.run(['xdotool','key','Tab'],env=ve,check=True)
  until=time.monotonic()+3;focused=False
  while time.monotonic()<until:
   subprocess.run(['import','-window',wid,str(protocol/'keyboard-end-focus.png')],env=ve,check=True)
   image=Image.open(protocol/'keyboard-end-focus.png').convert('RGB')
   focused=image.getpixel((image.width-117,11))==(255,255,255)
   if focused:break
   time.sleep(.02)
  assert focused,'view-only focus ring absent'
  subprocess.run(['xdotool','key','space'],env=ve,check=True)
  assert test.wait(timeout=3)==0,(protocol/'test.log').read_text()
  print('PASS protocol fit/resize/expose, received1:1/pan/restore and keyboard-focused local End',flush=True)
 subprocess.run([str(out/'viewer.test'),'-test.run','^TestDesktopViewX11FrameCacheOwnership$','-test.v','-test.timeout=15s'],env=pe,check=True)
 # A valid X server with an unsupported pixel format must fail closed.
 with open(out/'depth-display','w') as fd,open(out/'depth-xvfb.log','w') as log:
  depth=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','800x600x16'],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
 try:
  until=time.monotonic()+3
  while not (out/'depth-display').read_text().strip() and time.monotonic()<until:time.sleep(.02)
  number=(out/'depth-display').read_text().strip();assert number.isdecimal(),'unsupported-format fixture failed to start'
  subprocess.run([str(out/'viewer.test'),'-test.run','^TestDesktopViewX11UnsupportedDepth$','-test.v'],env=ve|{'DISPLAY':':'+number,'MNET_VIEWER_UNSUPPORTED_DEPTH':'1'},check=True)
 finally:depth.terminate();depth.wait()
 # Actual host app landmark, not a painted viewer-side mock.
 app=subprocess.Popen(['xmessage','-geometry','+100+200','-buttons','Fixture:0','Host desktop fixture\nView-only stream. No remote keyboard or pointer permission.'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 try:
  for mode in ['local-end','pending-close','decline','host-end','authority-loss','display-loss']:
   case=out/mode;case.mkdir()
   runenv=env|{'MNET_ATTENDED_ARTIFACTS':str(case),'MNET_VIEWER_CASE':mode}
   with open(case/'test.log','w') as log:
    test=subprocess.Popen([str(out/'host.test'),'-test.run','^TestAttendedX11BuiltViewOnlyClient$','-test.v','-test.timeout=25s'],env=runenv,stdout=log,stderr=log)
    deadline=time.monotonic()+18;approved=False;captured=False
    while test.poll() is None and time.monotonic()<deadline:
     stage=(case/'stage').read_text() if (case/'stage').exists() else ''
     if stage=='view' and not approved:
      host=window(env,'^Mnet: allow viewing');viewer=window(ve,'^Mnet view only:')
      if host and viewer:
       subprocess.run(['import','-window',host,str(case/'host-approval.png')],env=env,check=True)
       subprocess.run(['import','-window',viewer,str(case/'waiting.png')],env=ve,check=True)
       waiting_image=Image.open(case/'waiting.png').convert('RGB')
       assert waiting_image.getpixel((waiting_image.width-80,15))==(155,52,62),'local End pixels absent before approval'
       assert len(waiting_image.getcolors(1024*768) or [])<10,'preapproval image not blank'
       if mode=='pending-close':
        subprocess.run(['xdotool','windowfocus',viewer,'mousemove','--window',viewer,'896' if size=='1024x768x24' else '960','30','click','1'],env=ve,check=True);captured=True
       elif mode=='decline':
        subprocess.run(['xdotool','windowfocus',host,'key','Return'],env=env,check=True);captured=True
       else:subprocess.run(['xdotool','windowfocus',host,'key','Tab','Return'],env=env,check=True)
       approved=True
     if stage=='live' and not captured:
      viewer=window(ve,'^Mnet view only:.*Viewing.');active=window(env,'^Mnet: desktop sharing')
      if viewer and active:
       subprocess.run(['import','-window',viewer,str(case/'live.png')],env=ve,check=True)
       image=Image.open(case/'live.png').convert('RGB');colors=image.getcolors(1024*768) or [];blue=sum(n for n,c in colors if c[2]>c[0]+30 and c[1]>c[0]+10)
       if blue>10000:
        subprocess.run(['import','-window',active,str(case/'host-active.png')],env=env,check=True)
        if mode=='local-end':
         landmark=window(env,'^xmessage$');assert landmark,'owned host landmark absent'
         subprocess.run(['xdotool','windowmove',landmark,'400','400'],env=env,check=True)
         changed=False;until=time.monotonic()+3
         while time.monotonic()<until:
          subprocess.run(['import','-window',viewer,str(case/'delta.png')],env=ve,check=True)
          changed=ImageChops.difference(image.crop((0,72,image.width,image.height)),Image.open(case/'delta.png').convert('RGB').crop((0,72,image.width,image.height))).getbbox() is not None
          if changed:break
          time.sleep(.04)
         assert changed,'actual changed desktop did not reach native viewer'
         subprocess.run(['xdotool','windowfocus',viewer,'key','1'],env=ve,check=True)
         until=time.monotonic()+3;native_one=False
         while time.monotonic()<until:
          title=subprocess.check_output(['xdotool','getwindowname',viewer],env=ve,text=True)
          # The screen itself is inspected too; key processing is serialized
          # before the next callback/event, without any remote input request.
          subprocess.run(['import','-window',viewer,str(case/'one-to-one.png')],env=ve,check=True)
          native_one='[1:1 received]' in title
          if native_one:break
          time.sleep(.02)
         assert native_one,'native one-to-one window unavailable'
         subprocess.run(['xdotool','key','f','a','mousemove','--window',viewer,'896' if size=='1024x768x24' else '960','30','click','1'],env=ve,check=True)
        elif mode=='host-end':subprocess.run(['xdotool','windowfocus',active,'key','Return'],env=env,check=True)
        elif mode=='authority-loss':(case/'withdraw-authority').write_text('withdraw')
        elif mode=='display-loss':xs[1].terminate();xs[1].wait()
        captured=True
     time.sleep(.04)
    result=test.wait(timeout=3);print(mode,case,flush=True);print((case/'test.log').read_text());print((case/'viewer.log').read_text());assert result==0 and approved and captured
    assert not window(env,'^Mnet: allow viewing') and not window(env,'^Mnet: desktop sharing'),'host prompt survived retired session'
 finally:
  app.terminate();app.communicate(timeout=3)

finally:
 if test and test.poll() is None:test.kill();test.wait()
 for x in xs:x.terminate();x.wait()
