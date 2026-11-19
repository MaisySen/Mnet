#!/usr/bin/env python3
"""Independent Shared Desk TLS/capture/native-input fixture, no physical acceptance."""
import os,pathlib,shutil,subprocess,tempfile,time
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for name in ('go','Xvfb','import'):
 if not shutil.which(name):raise SystemExit('missing fixture dependency: '+name)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-native.'));print(out,flush=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'desk.test'),'./cmd/mnetd'],check=True)
xs=[];test=None
try:
 displays=[]
 for role in ('source','target'):
  with open(out/(role+'-display'),'w') as fd,open(out/(role+'-xvfb.log'),'w') as log:
   xs.append(subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log))
  for _ in range(100):
   number=(out/(role+'-display')).read_text().strip()
   if number:break
   time.sleep(.02)
  if not number:raise AssertionError('disposable X display unavailable')
  displays.append(':'+number)
 env=os.environ|{'DISPLAY':displays[0],'MNET_DESK_NATIVE_TEST':'1','MNET_DESK_TARGET_DISPLAY':displays[1],'MNET_DESK_ARTIFACTS':str(out)}
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
  test.wait(timeout=35)
  if test.returncode:raise AssertionError((out/'test.log').read_text())
 print((out/'test.log').read_text(),flush=True)
 print('PASS: pinned TLS 1.3 Desk pair, native source capture, independent recipient key/button/pointer, joined stop, source reclaim, surviving-child coordinator-loss cleanup. Virtual displays only.',flush=True)
finally:
 if test is not None and test.poll() is None:test.kill();test.wait()
 for process in xs:process.terminate();process.wait()
