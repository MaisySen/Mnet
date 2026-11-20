#!/usr/bin/env python3
"""Independent Shared Desk TLS/capture/native-input fixture, no physical acceptance."""
import os,pathlib,shutil,subprocess,tempfile,time
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for name in ('go','Xvfb','import','xdotool','zenity'):
 if not shutil.which(name):raise SystemExit('missing fixture dependency: '+name)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-native.'));print(out,flush=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'desk.test'),'./cmd/mnetd'],check=True)
xs=[];test=None
try:
 displays=[]
 target_size=os.environ.get('MNET_DESK_TARGET_SIZE','1024x768x24')
 if target_size not in ('1024x768x24','1280x960x24','2048x1536x24'):raise SystemExit('unsupported disposable target size')
 for role in ('source','target'):
  with open(out/(role+'-display'),'w') as fd,open(out/(role+'-xvfb.log'),'w') as log:
   xs.append(subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0',target_size if role=='target' else '1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log))
  for _ in range(100):
   number=(out/(role+'-display')).read_text().strip()
   if number:break
   time.sleep(.02)
  if not number:raise AssertionError('disposable X display unavailable')
  displays.append(':'+number)
 env=os.environ|{'DISPLAY':displays[0],'MNET_DESK_NATIVE_TEST':'1','MNET_DESK_TARGET_DISPLAY':displays[1],'MNET_DESK_ARTIFACTS':str(out),'NO_AT_BRIDGE':'1','DBUS_SESSION_BUS_ADDRESS':'unix:path=/nonexistent-mnet-fixture'}
 env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 with open(out/'test.log','w') as log:
  test=subprocess.Popen([str(out/'desk.test'),'-test.run=^TestDeskX11','-test.v','-test.timeout=40s'],env=env,stdout=log,stderr=log)
  until=time.monotonic()+10
  while not (out/'held.stage').exists():
   if test.poll() is not None:raise AssertionError((out/'test.log').read_text())
   if time.monotonic()>until:raise AssertionError('checked native stage not reached')
   time.sleep(.01)
  for role,display in zip(('source','target'),displays):
   subprocess.run(['import','-display',display,'-window','root',str(out/(role+'-held.png'))],check=True)
  (out/'held.captured').write_text('both actual display roots captured')
  until=time.monotonic()+15
  while not (out/'return.stage').exists():
   if test.poll() is not None:raise AssertionError((out/'test.log').read_text())
   if time.monotonic()>until:raise AssertionError('mapped return stage not reached')
   time.sleep(.01)
  subprocess.run(['import','-display',displays[0],'-window','root',str(out/'source-return.png')],check=True)
  (out/'return.captured').write_text('actual local return pixels captured')
  handled=set();until=time.monotonic()+30
  while test.poll() is None:
   for role,display in zip(('source','target'),displays):
    for denial in ('decline','withdraw'):
     marker='denial-'+denial
     if marker in handled:continue
     title='^Mnet Shared Desk: review recipient / '+denial+'$'
     found=subprocess.run(['xdotool','search','--onlyvisible','--name',title],env=env|{'DISPLAY':display},capture_output=True,text=True)
     if found.returncode==0 and found.stdout.strip():
      wid=found.stdout.strip().splitlines()[-1]
      subprocess.run(['import','-display',display,'-window',wid,str(out/('review-'+denial+'.png'))],check=True)
      if denial=='decline':subprocess.run(['xdotool','windowfocus',wid,'key','Return'],env=env|{'DISPLAY':display},check=True)
      else:(out/'review-withdraw.visible').write_text('native local review visible')
      handled.add(marker)
    for review in ('recipient','primary'):
     if review in handled:continue
     found=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet Shared Desk: review '+review+'$'],env=env|{'DISPLAY':display},capture_output=True,text=True)
     if found.returncode==0 and found.stdout.strip():
      wid=found.stdout.strip().splitlines()[-1]
      subprocess.run(['import','-display',display,'-window',wid,str(out/('review-'+review+'.png'))],check=True)
      subprocess.run(['xdotool','windowfocus',wid,'key','alt+a'],env=env|{'DISPLAY':display},check=True)
      closed_until=time.monotonic()+2
      while True:
       visible=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet Shared Desk: review '+review+'$'],env=env|{'DISPLAY':display},capture_output=True,text=True)
       if visible.returncode!=0 or not visible.stdout.strip():break
       if time.monotonic()>closed_until:raise AssertionError('actual review dialog did not close after local approval: '+review)
       time.sleep(.01)
      handled.add(review)
      if review=='primary':(out/'primary-review.allowed').write_text('local fixture approval dispatched')
   if time.monotonic()>until:raise AssertionError('native reviewed journey did not finish')
   time.sleep(.01)
  if handled!={'primary','recipient','denial-decline','denial-withdraw'}:raise AssertionError('both actual local review dialogs were not exercised')
  if test.returncode:raise AssertionError((out/'test.log').read_text())
 print((out/'test.log').read_text(),flush=True)
 print('PASS: pinned TLS 1.3 Desk pair, native source capture, independent recipient key/button/pointer, joined stop, source reclaim, source-map retirement/unchanged-map routing, armed-map no-create, surviving-child coordinator-loss cleanup. Virtual displays only.',flush=True)
finally:
 if test is not None and test.poll() is None:test.kill();test.wait()
 for process in xs:process.terminate();process.wait()
