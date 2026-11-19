#!/usr/bin/env python3
"""Attended recipient-local failure cases on an owned disposable X display."""
import os,subprocess,pathlib,tempfile,time,ctypes,json,signal,shutil
repo=str(pathlib.Path(__file__).resolve().parent.parent)
for name in ['go','Xvfb']:
 if shutil.which(name) is None:raise SystemExit('Missing isolated fixture dependency: '+name)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-recipient-guardian.'))
subprocess.run(['go','test','-race','-c','-o',str(out/'attended.test'),'./cmd/mnetd'],cwd=repo,check=True)
subprocess.run(['go','build','-o',str(out/'mnetd'),'./cmd/mnetd'],cwd=repo,check=True)
print('Artifacts: '+str(out),flush=True)
with open(out/'display','w') as fd,open(out/'xvfb.log','w') as log:
 x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
children=[]
try:
 for _ in range(100):
  number=(out/'display').read_text().strip()
  if number:break
  if x.poll() is not None:raise RuntimeError('display startup failed')
  time.sleep(.02)
 assert number.isdecimal() and x.poll() is None, 'no owned disposable display'
 (out/'Xauthority').touch(mode=0o600)
 env=os.environ|{'XAUTHORITY':str(out/'Xauthority'),'DISPLAY':':'+number,'MNET_ATTENDED_X11_TEST':'1','MNET_ATTENDED_ARTIFACTS':str(out),'MNET_ATTENDED_TEST_EXECUTABLE':str(out/'mnetd')}
 env.pop('WAYLAND_DISPLAY',None)
 lib=ctypes.CDLL('libX11.so.6');lib.XOpenDisplay.argtypes=[ctypes.c_char_p];lib.XOpenDisplay.restype=ctypes.c_void_p
 d=lib.XOpenDisplay(env['DISPLAY'].encode());assert d
 lib.XDefaultRootWindow.argtypes=[ctypes.c_void_p];lib.XDefaultRootWindow.restype=ctypes.c_ulong
 root=lib.XDefaultRootWindow(d)
 lib.XInternAtom.argtypes=[ctypes.c_void_p,ctypes.c_char_p,ctypes.c_int];lib.XInternAtom.restype=ctypes.c_ulong
 lib.XGetSelectionOwner.argtypes=[ctypes.c_void_p,ctypes.c_ulong];lib.XGetSelectionOwner.restype=ctypes.c_ulong
 owner_atom=lib.XInternAtom(d,b'_MNET_ATTENDED_INJECTOR_OWNER_V1',0)
 lib.XQueryPointer.argtypes=[ctypes.c_void_p,ctypes.c_ulong]+[ctypes.c_void_p]*7
 lib.XSync.argtypes=[ctypes.c_void_p,ctypes.c_int]
 xt=ctypes.CDLL('libXtst.so.6');xt.XTestFakeButtonEvent.argtypes=[ctypes.c_void_p,ctypes.c_uint,ctypes.c_int,ctypes.c_ulong]
 def press(down):xt.XTestFakeButtonEvent(d,1,int(down),0);lib.XSync(d,0)
 def held():
  wr=ctypes.c_ulong();cr=ctypes.c_ulong();rx=ctypes.c_int();ry=ctypes.c_int();wx=ctypes.c_int();wy=ctypes.c_int();mask=ctypes.c_uint()
  assert lib.XQueryPointer(d,root,ctypes.byref(wr),ctypes.byref(cr),ctypes.byref(rx),ctypes.byref(ry),ctypes.byref(wx),ctypes.byref(wy),ctypes.byref(mask))
  return bool(mask.value&256)
 def wait_held(want):
  until=time.monotonic()+1
  while time.monotonic()<until:
   if held()==want:return
   time.sleep(.01)
  raise AssertionError('unexpected held state '+str(held()))
 subprocess.run([str(out/'attended.test'),'-test.run','^TestAttendedX11ProcessCloseAndForeignPreheld$','-test.v','-test.timeout=15s'],env=env,check=True)
 def start_coordinator(name):
  ready=out/'coordinator-ready.json';ready.unlink(missing_ok=True)
  p=subprocess.Popen([str(out/'attended.test'),'-test.run','^TestAttendedX11ProcessHarness$','-test.timeout=30s'],env=env|{'MNET_PROCESS_FIXTURE_ROLE':'coordinator'},stdout=open(out/(name+'.stdout'),'w'),stderr=open(out/(name+'.stderr'),'w'),start_new_session=True);children.append(p)
  until=time.monotonic()+3
  while not ready.exists():
   if p.poll() is not None or time.monotonic()>until:raise RuntimeError('coordinator not ready, inspect '+str(out))
   time.sleep(.01)
  data=json.loads(ready.read_text())
  return p,data['Injector'],['ready','down' if data['Accepted'] else 'refused-preheld']
 p,pid,states=start_coordinator('coordinator-kill');assert states==['ready','down'];wait_held(True)
 start=time.monotonic();os.kill(p.pid,signal.SIGKILL);p.wait();wait_held(False)
 print('PASS coordinator SIGKILL: independently surviving injector released owned left button in %.3fs'%(time.monotonic()-start),flush=True)
 p,pid,states=start_coordinator('coordinator-stop');assert states==['ready','down'];wait_held(True)
 os.kill(p.pid,signal.SIGSTOP);start=time.monotonic()
 assert lib.XGetSelectionOwner(d,owner_atom)!=0, 'child ownership died with stopped coordinator'
 while held() and time.monotonic()-start<3:time.sleep(.01)
 assert not held()
 until=time.monotonic()+1
 while lib.XGetSelectionOwner(d,owner_atom)!=0 and time.monotonic()<until:time.sleep(.01)
 assert lib.XGetSelectionOwner(d,owner_atom)==0, 'child failed to release its owner after input cleanup'
 print('PASS coordinator SIGSTOP: independent2s lease expired and released own button after %.3fs'%(time.monotonic()-start),flush=True)
 os.kill(p.pid,signal.SIGKILL);p.wait()
 p,pid,states=start_coordinator('injector-kill');assert states==['ready','down'];wait_held(True)
 os.kill(pid,signal.SIGKILL);time.sleep(.2);assert held()
 print('NEGATIVE injector SIGKILL: left button remained held, no claim of general crash recovery',flush=True)
 os.kill(p.pid,signal.SIGKILL);p.wait();press(False)
 press(True);p,pid,states=start_coordinator('foreign-preheld');assert states==['ready','refused-preheld'];assert held()
 os.kill(p.pid,signal.SIGKILL);p.wait();time.sleep(.2);assert held()
 print('PASS foreign preheld: injector refused ownership and pipe-loss cleanup did not release external held button',flush=True)
 press(False)
finally:
 for p in children:
  try:os.killpg(p.pid,signal.SIGKILL)
  except ProcessLookupError:pass
  p.wait()
 x.terminate();x.wait()
