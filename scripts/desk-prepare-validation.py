#!/usr/bin/env python3
"""Actual one-run engineering CLI processes on two disposable X servers."""
import ctypes,ctypes.util,json,os,pathlib,shutil,signal,socket,ssl,subprocess,tempfile,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for dep in ('go','Xvfb','xdotool','xrandr','import','zenity'):
 if not shutil.which(dep):raise SystemExit('missing dependency: '+dep)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-prepare.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnetd'),'./cmd/mnetd'],check=True)
subprocess.run(['go','build','-race','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'fixture.test'),'./cmd/mnetd'],check=True)
# Own orphaned disposable grandchildren during intentional launcher crashes.
# Reap only exact native child PIDs observed in this fixture, never other jobs.
libc=ctypes.CDLL(None,use_errno=True)
if libc.prctl(36,1,0,0,0)!=0:raise OSError(ctypes.get_errno(),'fixture subreaper unavailable')
xs=[];children=[];display_handles=[];native_children=set()
x=ctypes.CDLL(ctypes.util.find_library('X11'));x.XOpenDisplay.argtypes=[ctypes.c_char_p];x.XOpenDisplay.restype=ctypes.c_void_p
x.XDefaultRootWindow.argtypes=[ctypes.c_void_p];x.XDefaultRootWindow.restype=ctypes.c_ulong
x.XQueryKeymap.argtypes=[ctypes.c_void_p,ctypes.c_void_p]
x.XQueryPointer.argtypes=[ctypes.c_void_p,ctypes.c_ulong,ctypes.c_void_p,ctypes.c_void_p,ctypes.c_void_p,ctypes.c_void_p,ctypes.c_void_p,ctypes.c_void_p,ctypes.c_void_p]
x.XCloseDisplay.argtypes=[ctypes.c_void_p]
x.XUnmapWindow.argtypes=[ctypes.c_void_p,ctypes.c_ulong]
x.XMapWindow.argtypes=[ctypes.c_void_p,ctypes.c_ulong]
def hide_status(index,wid,transient):
 x.XUnmapWindow(display_handles[index],int(wid))
 if transient:x.XMapWindow(display_handles[index],int(wid))
 x.XSync(display_handles[index],0)
x.XGrabKeyboard.argtypes=[ctypes.c_void_p,ctypes.c_ulong,ctypes.c_int,ctypes.c_int,ctypes.c_int,ctypes.c_ulong]
x.XUngrabKeyboard.argtypes=[ctypes.c_void_p,ctypes.c_ulong]
x.XGrabPointer.argtypes=[ctypes.c_void_p,ctypes.c_ulong,ctypes.c_int,ctypes.c_uint,ctypes.c_int,ctypes.c_int,ctypes.c_ulong,ctypes.c_ulong,ctypes.c_ulong]
x.XUngrabPointer.argtypes=[ctypes.c_void_p,ctypes.c_ulong]
x.XSync.argtypes=[ctypes.c_void_p,ctypes.c_int]
x.XGetKeyboardMapping.argtypes=[ctypes.c_void_p,ctypes.c_ubyte,ctypes.c_int,ctypes.POINTER(ctypes.c_int)];x.XGetKeyboardMapping.restype=ctypes.POINTER(ctypes.c_ulong)
x.XChangeKeyboardMapping.argtypes=[ctypes.c_void_p,ctypes.c_int,ctypes.c_int,ctypes.POINTER(ctypes.c_ulong),ctypes.c_int]
x.XFree.argtypes=[ctypes.c_void_p]
def change_keymap(d):
 width=ctypes.c_int();raw=x.XGetKeyboardMapping(d,38,1,ctypes.byref(width))
 if not raw or width.value<1 or width.value>255:raise AssertionError('fixture original keymap unavailable')
 original=(ctypes.c_ulong*width.value)(*[raw[i] for i in range(width.value)]);x.XFree(raw)
 changed=(ctypes.c_ulong*width.value)(*original);changed[0]=ord('z') if original[0]!=ord('z') else ord('x')
 x.XChangeKeyboardMapping(d,38,width.value,changed,1);x.XSync(d,0)
 def restore():x.XChangeKeyboardMapping(d,38,width.value,original,1);x.XSync(d,0)
 return restore
def poll(check,label,seconds=5):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  value=check()
  if value:return value
  time.sleep(.01)
 raise AssertionError('deadline: '+label)
def keysbuttons(d,button_mask=256):
 keys=ctypes.create_string_buffer(32);x.XQueryKeymap(d,keys)
 a=ctypes.c_ulong();b=ctypes.c_ulong();rx=ctypes.c_int();ry=ctypes.c_int();wx=ctypes.c_int();wy=ctypes.c_int();mask=ctypes.c_uint()
 x.XQueryPointer(d,x.XDefaultRootWindow(d),ctypes.byref(a),ctypes.byref(b),ctypes.byref(rx),ctypes.byref(ry),ctypes.byref(wx),ctypes.byref(wy),ctypes.byref(mask))
 return bool(keys.raw[38//8]&(1<<(38%8))),bool(mask.value&button_mask)
def native_pointer(d):
 a=ctypes.c_ulong();b=ctypes.c_ulong();rx=ctypes.c_int();ry=ctypes.c_int();wx=ctypes.c_int();wy=ctypes.c_int();mask=ctypes.c_uint()
 if not x.XQueryPointer(d,x.XDefaultRootWindow(d),ctypes.byref(a),ctypes.byref(b),ctypes.byref(rx),ctypes.byref(ry),ctypes.byref(wx),ctypes.byref(wy),ctypes.byref(mask)):raise AssertionError('native pointer unavailable')
 return rx.value,ry.value
def command(display,*args):subprocess.run(['xdotool',*args],env=env|{'DISPLAY':display},check=True)
def approve(role,display,action='alt+a',name=''):
 title='^Mnet Shared Desk: review '+role+'$'
 def find():
  result=subprocess.run(['xdotool','search','--onlyvisible','--name',title],env=env|{'DISPLAY':display},capture_output=True,text=True)
  return result.stdout.strip().splitlines()[-1] if result.returncode==0 and result.stdout.strip() else None
 wid=poll(find,role+' review visible')
 def rendered():
  subprocess.run(['import','-display',display,'-window',wid,str(out/('launch-review-'+role+name+'.png'))],check=True)
  image=Image.open(out/('launch-review-'+role+name+'.png')).convert('RGB')
  return len(image.getcolors(image.width*image.height) or [])>16
 poll(rendered,role+' actual review pixels painted')
 command(display,'windowfocus',wid,'key',action)
 poll(lambda:not find(),role+' review closed')
def live_window(role,display,state=None):
 pattern='^Mnet Shared Desk: live '+role+(r' \['+state+r'\]$' if state else '')
 result=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env|{'DISPLAY':display},capture_output=True,text=True)
 return result.stdout.strip().splitlines()[-1] if result.returncode==0 and result.stdout.strip() else None

def live_capture(role,display,state,suffix=''):
 wid=poll(lambda:live_window(role,display,state),role+' '+state+' live status')
 subprocess.run(['import','-display',display,'-window',wid,str(out/('live-'+role+'-'+state+suffix+'.png'))],check=True)
 return wid

try:
 displays=[]
 size=os.environ.get('MNET_DESK_TARGET_SIZE','1024x768x24')
 if size not in ('1024x768x24','1280x960x24'):raise AssertionError('unsupported disposable size')
 for role in ('primary','recipient'):
  with open(out/(role+'-display'),'w') as fd,open(out/(role+'-x.log'),'w') as log:
   xs.append(subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0',size if role=='recipient' else '1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log))
  number=poll(lambda:(out/(role+'-display')).read_text().strip(),role+' X display')
  displays.append(':'+number)
 env=os.environ|{'DISPLAY':displays[0],'MNET_DESK_TARGET_DISPLAY':displays[1],'MNET_DESK_ARTIFACTS':str(out),'MNET_DESK_LAUNCH_PREPARE':'1','NO_AT_BRIDGE':'1','DBUS_SESSION_BUS_ADDRESS':'unix:path=/nonexistent-mnet-fixture'}
 env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 subprocess.run([str(out/'fixture.test'),'-test.run=^TestDeskX11PrepareLauncher$','-test.v','-test.timeout=10s'],env=env,check=True)
 inspections={}
 for role,display in zip(('primary','recipient'),displays):
  inspected=subprocess.run([str(out/'mnetd'),'-desk-inspect',str(out/role/'cert.pem')],env=env|{'DISPLAY':display},capture_output=True,text=True,timeout=5,check=True)
  evidence=json.loads(inspected.stdout)
  inspections[role]=evidence
  spec=json.loads((out/role/'run.json').read_text())
  expected=spec['binding']['primary_key' if role=='primary' else 'recipient_key']
  actual=next(d for d in spec['layout']['displays'] if d['device']==expected)
  if evidence!={'local_evidence_only':True,'approval':False,'certificate_key':expected,'keyboard_hash':spec['binding']['keyboard_hash'],'pixel_width':actual['pixel_width'],'pixel_height':actual['pixel_height']}:raise AssertionError('read-only inspection differs')
  if list((out/role/'runs').iterdir()):raise AssertionError('read-only inspect reserved a run')
  incompatible=subprocess.run([str(out/'mnetd'),'-desk-run',str(out/role/'run.json'),'-up'],env=env|{'DISPLAY':display},capture_output=True,text=True,timeout=5)
  if incompatible.returncode==0 or 'cannot combine' not in incompatible.stderr:raise AssertionError('launcher mixed with daemon flags')
 print('PASS actual read-only local inspection and incompatible daemon-mode refusal',flush=True)
 request={'version':1,'primary':{'inspection':inspections['primary'],'certificate_file':str(out/'primary'/'cert.pem'),'private_key_file':str(out/'primary'/'key.pem')},'recipient':{'inspection':inspections['recipient'],'certificate_file':str(out/'recipient'/'cert.pem'),'private_key_file':str(out/'recipient'/'key.pem')},'recipient_address':spec['address'],'source_edge':'right','lifetime_seconds':30}
 request_path=out/'preparation.json';request_path.write_text(json.dumps(request))
 drafts=out/'prepared'
 subprocess.run([str(out/'mnet'),'desk','prepare',str(request_path),str(drafts)],env=env,check=True,timeout=5)
 for role in ('primary','recipient'):
  original=(drafts/role/'run.json').read_bytes()
  generated=json.loads(original)
  assert generated['binding']['primary_key']==inspections['primary']['certificate_key']
  assert generated['binding']['recipient_key']==inspections['recipient']['certificate_key']
  assert generated['role']==role and not list((drafts/role/'replay').iterdir())
 assert not live_window('recipient',displays[1]) and not live_window('primary',displays[0]),'draft preparation started live run'
 print('PASS built preparation uses actual independently supplied inspection; no key copy, replay reservation, live window or input before explicit launch',flush=True)

 for display in displays:
  d=x.XOpenDisplay(display.encode());assert d;display_handles.append(d)
 def launch(role):
  log=open(out/(role+'.log'),'w');p=subprocess.Popen([str(out/'mnetd'),'-desk-run',str(drafts/role/'run.json')],env=env|{'DISPLAY':displays[0 if role=='primary' else 1]},stdout=log,stderr=log);children.append(p);log.close();return p
 recipient=launch('recipient');approve('recipient',displays[1]);poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'recipient listening')
 live_capture('recipient',displays[1],'waiting')
 primary=launch('primary');approve('primary',displays[0])
 live_capture('primary',displays[0],'armed')
 # Fresh edge arrival, with no held input, not resting edge or an explicit test
 # call into capture. The actual CLI owns review, transport and native child.
 command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
 poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'actual launcher input active')
 command(displays[0],'keydown','a','mousedown','1')
 poll(lambda:keysbuttons(display_handles[1])==(True,True),'actual recipient owns source key/button')
 live_capture('primary',displays[0],'forwarding');live_capture('recipient',displays[1],'receiving')
 print('PASS actual primary/recipient daemon commands: local review, edge arrival, TLS pair and native held key/button',flush=True)
 # SIGTERM is joined by the actual entrypoint, not a fixture cleanup helper.
 primary.send_signal(signal.SIGTERM)
 primary.wait(timeout=5);recipient.wait(timeout=5)
 poll(lambda:keysbuttons(display_handles[1])==(False,False),'recipient release after launcher stop')
 command(displays[0],'keyup','a','mouseup','1')
 d=display_handles[0];root=x.XDefaultRootWindow(d)
 if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('source keyboard not reclaimed')
 x.XUngrabKeyboard(d,0)
 if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('source pointer not reclaimed')
 x.XUngrabPointer(d,0);x.XSync(d,0)
 if primary.returncode!=0 or recipient.returncode!=0:raise AssertionError('launcher teardown failure: '+(out/'primary.log').read_text()+(out/'recipient.log').read_text())
 print('PASS actual SIGTERM joins recipient native release and primary reclaim',flush=True)
 # Re-running identical config must refuse before any new prompt or socket.
 for role,display in zip(('primary','recipient'),displays):
  result=subprocess.run([str(out/'mnetd'),'-desk-run',str(drafts/role/'run.json')],env=env|{'DISPLAY':display},capture_output=True,text=True,timeout=5)
  if result.returncode==0 or 'already consumed' not in result.stderr:raise AssertionError('replayed config not refused')
 print('PASS persistent same-config run reuse refused on both actual commands',flush=True)

finally:
 for p in children:
  if p.poll() is None:p.kill();p.wait()
 for pid in native_children:
  try:os.waitpid(pid,0)
  except ChildProcessError:pass
 for d in display_handles:x.XCloseDisplay(d)
 # Never retain disposable TLS private keys in a CI artifact.
 for role in ('primary','recipient'):(out/role/'key.pem').unlink(missing_ok=True)
 for p in xs:p.terminate();p.wait()
