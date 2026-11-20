#!/usr/bin/env python3
"""Actual one-run engineering CLI processes on two disposable X servers."""
import ctypes,ctypes.util,json,os,pathlib,shutil,signal,socket,ssl,subprocess,tempfile,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for dep in ('go','Xvfb','xdotool','xrandr','import','zenity'):
 if not shutil.which(dep):raise SystemExit('missing dependency: '+dep)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-launch.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnetd'),'./cmd/mnetd'],check=True)
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
 for role,display in zip(('primary','recipient'),displays):
  inspected=subprocess.run([str(out/'mnetd'),'-desk-inspect',str(out/role/'cert.pem')],env=env|{'DISPLAY':display},capture_output=True,text=True,timeout=5,check=True)
  evidence=json.loads(inspected.stdout)
  spec=json.loads((out/role/'run.json').read_text())
  expected=spec['binding']['primary_key' if role=='primary' else 'recipient_key']
  actual=next(d for d in spec['layout']['displays'] if d['device']==expected)
  if evidence!={'local_evidence_only':True,'approval':False,'certificate_key':expected,'keyboard_hash':spec['binding']['keyboard_hash'],'pixel_width':actual['pixel_width'],'pixel_height':actual['pixel_height']}:raise AssertionError('read-only inspection differs')
  if list((out/role/'runs').iterdir()):raise AssertionError('read-only inspect reserved a run')
  incompatible=subprocess.run([str(out/'mnetd'),'-desk-run',str(out/role/'run.json'),'-up'],env=env|{'DISPLAY':display},capture_output=True,text=True,timeout=5)
  if incompatible.returncode==0 or 'cannot combine' not in incompatible.stderr:raise AssertionError('launcher mixed with daemon flags')
 print('PASS actual read-only local inspection and incompatible daemon-mode refusal',flush=True)
 for display in displays:
  d=x.XOpenDisplay(display.encode());assert d;display_handles.append(d)
 def launch(role):
  log=open(out/(role+'.log'),'w');p=subprocess.Popen([str(out/'mnetd'),'-desk-run',str(out/role/'run.json')],env=env|{'DISPLAY':displays[0 if role=='primary' else 1]},stdout=log,stderr=log);children.append(p);log.close();return p
 # Review is part of the run lifetime and owns a real external dialog.
 # Cancellation must join that dialog before any listener/native handoff.
 for role,display,token,mode in [('primary',displays[0],'e1','sigterm'),('recipient',displays[1],'e2','sigterm'),('primary',displays[0],'e3','deadline'),('recipient',displays[1],'e4','deadline')]:
  review_path=out/role/'run.json';original_spec=review_path.read_text();review_spec=json.loads(original_spec)
  review_spec['binding']['run']=token*32;review_spec['lifetime_seconds']=2 if mode=='deadline' else 30;review_path.write_text(json.dumps(review_spec))
  reviewing=launch(role)
  def review_window():
   found=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet Shared Desk: review '+role+'$'],env=env|{'DISPLAY':display},capture_output=True,text=True)
   return found.stdout.strip().splitlines()[-1] if found.returncode==0 and found.stdout.strip() else None
  review_id=poll(review_window,role+' pending review '+mode)
  review_image=out/('pending-review-'+role+'-'+mode+'.png')
  def review_painted():
   subprocess.run(['import','-display',display,'-window',review_id,str(review_image)],check=True)
   pixels=Image.open(review_image).convert('RGB')
   return len(pixels.getcolors(pixels.width*pixels.height) or [])>16
  poll(review_painted,role+' pending review painted')
  if mode=='sigterm':reviewing.send_signal(signal.SIGTERM)
  reviewing.wait(timeout=4)
  if reviewing.returncode==0 or reviewing.returncode<0:raise AssertionError('review cancellation bypassed joined refusal')
  poll(lambda:not review_window(),role+' review child joined')
  if live_window(role,display):raise AssertionError('unanswered review opened live status')
  if (pathlib.Path('/proc')/str(reviewing.pid)).exists():raise AssertionError('reviewing launcher survived wait')
  if not (out/role/'runs'/(token*32)).exists():raise AssertionError('review cancellation did not consume run')
  address=review_spec['address'].rsplit(':',1)
  with socket.socket() as probe:
   probe.settimeout(.2)
   if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('unanswered review opened listener')
  replay=subprocess.run([str(out/'mnetd'),'-desk-run',str(review_path)],env=env|{'DISPLAY':display},capture_output=True,text=True,timeout=3)
  if replay.returncode==0 or 'already consumed' not in replay.stderr:raise AssertionError('canceled review run reusable')
  review_path.write_text(original_spec)
  print('PASS actual '+role+' review '+mode+' joins unanswered dialog, consumes run and opens no live status/listener; no implicit approval',flush=True)
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
  result=subprocess.run([str(out/'mnetd'),'-desk-run',str(out/role/'run.json')],env=env|{'DISPLAY':display},capture_output=True,text=True,timeout=5)
  if result.returncode==0 or 'already consumed' not in result.stderr:raise AssertionError('replayed config not refused')
 print('PASS persistent same-config run reuse refused on both actual commands',flush=True)
 # Recipient SIGTERM is distinct from source shutdown or a status-window
 # Stop. Observe release before any source keyup, then inspect source grabs.
 for role in ('primary','recipient'):
  fresh_path=out/role/'run.json';fresh=json.loads(fresh_path.read_text());fresh['binding']['run']='04'*32;fresh_path.write_text(json.dumps(fresh))
 recipient=launch('recipient');approve('recipient',displays[1],name='-recipient-term')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'recipient SIGTERM listening')
 primary=launch('primary');approve('primary',displays[0],name='-recipient-term')
 command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
 poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'recipient SIGTERM active')
 command(displays[0],'keydown','a','mousedown','3')
 poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'recipient held key/right button before SIGTERM')
 recipient.send_signal(signal.SIGTERM);recipient.wait(timeout=5);primary.wait(timeout=5)
 poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'recipient SIGTERM released before source keyup')
 if recipient.returncode<0:raise AssertionError('recipient SIGTERM bypassed joined shutdown')
 d=display_handles[0];root=x.XDefaultRootWindow(d)
 if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('recipient SIGTERM retained source keyboard')
 x.XUngrabKeyboard(d,0)
 if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('recipient SIGTERM retained source pointer')
 x.XUngrabPointer(d,0);x.XSync(d,0)
 if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('recipient SIGTERM retained live window')
 address=json.loads((out/'recipient'/'run.json').read_text())['address'].rsplit(':',1)
 with socket.socket() as probe:
  probe.settimeout(.2)
  if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('recipient SIGTERM retained listener')
 command(displays[0],'keyup','a','mouseup','3')
 print('PASS actual recipient SIGTERM joins held native key/right-button release before source keyup, reclaims source and closes status/listener; exit codes primary=%d recipient=%d'%(primary.returncode,recipient.returncode),flush=True)
 # A new independently reviewed attempt uses the recipient's real local Stop
 # button while raw key38 and RIGHT button are held remotely. The local left
 # click therefore cannot itself release either held transition under test.
 for role in ('primary','recipient'):
  path=out/role/'run.json';fresh=json.loads(path.read_text());fresh['binding']['run']='c'*64;path.write_text(json.dumps(fresh))
 recipient=launch('recipient');approve('recipient',displays[1],name='-live-stop')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'live stop recipient listening')
 primary=launch('primary');approve('primary',displays[0],name='-live-stop')
 command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
 poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'live stop active')
 command(displays[0],'keydown','a','mousedown','3')
 poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'recipient key/right-button held before local Stop')
 stop_window=live_capture('recipient',displays[1],'receiving','-before-stop')
 command(displays[1],'windowraise',stop_window,'mousemove','--window',stop_window,'360','267','click','1')
 primary.wait(timeout=5);recipient.wait(timeout=5)
 poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'recipient local Stop joined key/right-button release')
 command(displays[0],'keyup','a','mouseup','3')
 d=display_handles[0];root=x.XDefaultRootWindow(d)
 if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('local Stop retained source keyboard')
 x.XUngrabKeyboard(d,0)
 if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('local Stop retained source pointer')
 x.XUngrabPointer(d,0);x.XSync(d,0)
 if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('ended run retained a live status window')
 print('PASS actual recipient Stop button joins native key/right-button release and source reclaim; lost remote ACK remains an error, not a clean-stop claim',flush=True)
 # A fresh run declined by default Return consumes its run but opens no port.
 path=out/'recipient'/'run.json';spec=json.loads(path.read_text());spec['binding']['run']='f'*64;path.write_text(json.dumps(spec))
 declined=launch('recipient');approve('recipient',displays[1],'Return','-decline');declined.wait(timeout=5)
 if declined.returncode==0 or 'declined' not in (out/'recipient.log').read_text():raise AssertionError('actual default decline accepted')
 if not (out/'recipient'/'runs'/('f'*64)).exists():raise AssertionError('decline did not consume run')
 print('PASS actual default decline terminal and token consumed; no implicit enrollment or fallback',flush=True)
 # A fresh recipient attempt must be stoppable while waiting, before there is
 # any TLS peer or native injector. The passive shortcut owns that interval.
 spec['binding']['run']='a'*64;path.write_text(json.dumps(spec))
 waiting=launch('recipient');approve('recipient',displays[1],name='-waiting-stop')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'waiting recipient listen')
 command(displays[1],'key','ctrl+alt+shift+F12');waiting.wait(timeout=5)
 if waiting.returncode==0 or 'canceled or accept failed' not in (out/'recipient.log').read_text():raise AssertionError('waiting recipient local stop not terminal')
 address=spec['address'].rsplit(':',1)
 with socket.socket() as probe:
  probe.settimeout(.2)
  if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('waiting local stop left listener open')
 print('PASS actual waiting recipient shortcut cancels run and closes listener before native input',flush=True)
 # The live waiting window can cancel without a peer or input owner.
 spec['binding']['run']='d'*64;path.write_text(json.dumps(spec))
 waiting=launch('recipient');approve('recipient',displays[1],name='-window-stop')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'window stop waiting listen')
 stop_window=live_capture('recipient',displays[1],'waiting','-before-stop')
 # An unrelated press cannot become Stop merely by releasing over its band.
 command(displays[1],'windowfocus',stop_window,'mousemove','--window',stop_window,'360','220','mousedown','1','mousemove','--window',stop_window,'360','260','mouseup','1')
 time.sleep(.05);assert waiting.poll() is None,'outside press became Stop on inside release'
 command(displays[1],'mousemove','--window',stop_window,'360','260','mousedown','1','mousemove','--window',stop_window,'360','220','mouseup','1')
 time.sleep(.05);assert waiting.poll() is None,'inside press stopped after outside release'
 # Losing keyboard focus retires a pending pointer activation too.
 command(displays[1],'mousemove','--window',stop_window,'360','260','mousedown','1','windowfocus',str(x.XDefaultRootWindow(display_handles[1])),'mouseup','1','windowfocus',stop_window)
 time.sleep(.05);assert waiting.poll() is None,'focus loss retained pending Stop'
 # Exercise the newly supported top of the full44px target, not only its center.
 command(displays[1],'mousemove','--window',stop_window,'360','242','click','1');waiting.wait(timeout=5)
 with socket.socket() as probe:
  probe.settimeout(.2)
  if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('status Stop left listener open')
 if live_window('recipient',displays[1]):raise AssertionError('waiting status window survived Stop')
 print('PASS actual waiting-recipient Stop button cancels without a peer and closes listener/window',flush=True)
 # Closing an armed source window withdraws before any pair connection or
 # source input grab. It cannot turn closing UI into a hidden enabled run.
 primary_path=out/'primary'/'run.json';armed_spec=json.loads(primary_path.read_text());armed_spec['binding']['run']='9'*64;primary_path.write_text(json.dumps(armed_spec))
 armed=launch('primary');approve('primary',displays[0],name='-armed-close')
 armed_window=live_capture('primary',displays[0],'armed','-before-close')
 command(displays[0],'windowclose',armed_window);armed.wait(timeout=5)
 if 'sending raw input' in (out/'primary.log').read_text() or live_window('primary',displays[0]):raise AssertionError('closed source window activated or remained live')
 print('PASS actual armed-source window close withdraws before activation, without connecting or grabbing input',flush=True)
 # A destroyed/nonconforming status surface must not leave hidden input
 # enabled. A new waiting run is resized, then must close its listener too.
 spec['binding']['run']='8'*64;path.write_text(json.dumps(spec))
 waiting=launch('recipient');approve('recipient',displays[1],name='-resize-stop')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'resize waiting listen')
 resize_window=live_capture('recipient',displays[1],'waiting','-before-resize')
 command(displays[1],'windowsize',resize_window,'400','180');waiting.wait(timeout=5)
 with socket.socket() as probe:
  probe.settimeout(.2)
  if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('unsupported status resize left listener open')
 if live_window('recipient',displays[1]):raise AssertionError('resized status window survived cancellation')
 print('PASS actual unsupported status resize retires waiting run and listener, without hidden fallback',flush=True)
 # Keyboard-only Stop must use a matching focused Enter/Space release.
 spec['binding']['run']='7'*64;path.write_text(json.dumps(spec))
 waiting=launch('recipient');approve('recipient',displays[1],name='-keyboard-stop')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'keyboard waiting listen')
 stop_window=live_capture('recipient',displays[1],'waiting','-keyboard-stop')
 command(displays[1],'windowfocus',stop_window,'key','Tab')
 def stop_focus_pixels():
  subprocess.run(['import','-display',displays[1],'-window',stop_window,str(out/'live-recipient-keyboard-focus.png')],check=True)
  return Image.open(out/'live-recipient-keyboard-focus.png').convert('RGB').getpixel((21,243))==(255,255,255)
 poll(stop_focus_pixels,'actual keyboard Stop focus pixels')
 command(displays[1],'keyup','Return')
 time.sleep(.05);assert waiting.poll() is None,'unpressed Return stopped run'
 command(displays[1],'keydown','Return','windowfocus',str(x.XDefaultRootWindow(display_handles[1])),'keyup','Return','windowfocus',stop_window)
 time.sleep(.05);assert waiting.poll() is None,'focus loss retained pending keyboard Stop'
 command(displays[1],'key','Tab','keydown','Return','key','Tab','keyup','Return')
 time.sleep(.05);assert waiting.poll() is None,'Tab retained pending keyboard Stop'
 command(displays[1],'key','space');waiting.wait(timeout=5)
 with socket.socket() as probe:
  probe.settimeout(.2)
  if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('keyboard Stop left listener open')
 if live_window('recipient',displays[1]):raise AssertionError('keyboard Stop left status window open')
 print('PASS actual keyboard Stop focus pixels, exact release, focus-loss/Tab cancellation and joined listener/window teardown',flush=True)
 # Keyboard Stop also joins the native recipient while independent source
 # key38 and right button remain held. Local Space cannot release either.
 for role in ('primary','recipient'):
  fresh_path=out/role/'run.json';fresh=json.loads(fresh_path.read_text());fresh['binding']['run']='6'*64;fresh_path.write_text(json.dumps(fresh))
 recipient=launch('recipient');approve('recipient',displays[1],name='-keyboard-held')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'keyboard held recipient listening')
 primary=launch('primary');approve('primary',displays[0],name='-keyboard-held')
 command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
 poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'keyboard Stop source active')
 command(displays[0],'keydown','a','mousedown','3')
 poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'keyboard Stop recipient owns key/right button')
 stop_window=live_capture('recipient',displays[1],'receiving','-keyboard-held')
 command(displays[1],'windowfocus',stop_window,'key','Tab');poll(stop_focus_pixels,'receiving keyboard Stop focus pixels')
 command(displays[1],'key','space');primary.wait(timeout=5);recipient.wait(timeout=5)
 poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'keyboard Stop releases native held key/right button')
 command(displays[0],'keyup','a','mouseup','3')
 d=display_handles[0];root=x.XDefaultRootWindow(d)
 if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('keyboard Stop retained source keyboard')
 x.XUngrabKeyboard(d,0)
 if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('keyboard Stop retained source pointer')
 x.XUngrabPointer(d,0);x.XSync(d,0)
 if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('keyboard Stop retained live window')
 print('PASS actual receiving keyboard Stop joins held native key/right-button release and source reclaim',flush=True)
 # The real launcher must return across the approved local edge, not only
 # through Stop. A held native key/right button blocks that crossing.
 for role in ('primary','recipient'):
  fresh_path=out/role/'run.json';fresh=json.loads(fresh_path.read_text());fresh['binding']['run']='5'*64;fresh_path.write_text(json.dumps(fresh))
 recipient=launch('recipient');approve('recipient',displays[1],name='-edge-return')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'return recipient listening')
 primary=launch('primary');approve('primary',displays[0],name='-edge-return')
 command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
 poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'return source active')
 command(displays[0],'keydown','a','mousedown','3')
 poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'held input before return')
 command(displays[0],'mousemove','0','384')
 poll(lambda:native_pointer(display_handles[1])[0]==0,'held recipient left boundary')
 time.sleep(.05);assert primary.poll() is None and recipient.poll() is None,'held edge returned without release'
 assert keysbuttons(display_handles[1],1024)==(True,True),'held return lost native ledger'
 live_capture('primary',displays[0],'forwarding','-held-return');live_capture('recipient',displays[1],'receiving','-held-return')
 command(displays[0],'keyup','a','mouseup','3')
 poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'release before local return')
 command(displays[0],'mousemove','507','384')
 primary.wait(timeout=5);recipient.wait(timeout=5)
 poll(lambda:native_pointer(display_handles[0])==(1023,300),'actual mapped local pointer after joined return')
 if primary.returncode!=0 or recipient.returncode!=0:raise AssertionError('actual launcher edge return reported cleanup failure')
 if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('edge return left live status')
 subprocess.run(['import','-display',displays[0],'-window','root',str(out/'actual-launcher-local-return.png')],check=True)
 d=display_handles[0];root=x.XDefaultRootWindow(d)
 if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('edge return retained source keyboard')
 x.XUngrabKeyboard(d,0)
 if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('edge return retained source pointer')
 x.XUngrabPointer(d,0);x.XSync(d,0)
 command(displays[0],'mousemove','500','300','mousemove','1023','300')
 time.sleep(.05)
 if primary.poll() is None or recipient.poll() is None:raise AssertionError('edge return automatically reactivated consumed run')
 print('PASS actual launcher held edge refuses return, released edge joins recipient and restores exact local pixel, no auto-reactivation',flush=True)
 # Actual source-root resize must end both one-run launchers, not merely a
 # direct fixture capture. The recipient owns held key/right-button ledger.
 for role in ('primary','recipient'):
  fresh_path=out/role/'run.json';fresh=json.loads(fresh_path.read_text());fresh['binding']['run']='2'*64;fresh_path.write_text(json.dumps(fresh))
 recipient=launch('recipient');approve('recipient',displays[1],name='-source-resize')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'source resize recipient listening')
 primary=launch('primary');approve('primary',displays[0],name='-source-resize')
 command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
 poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'source resize active')
 command(displays[0],'keydown','a','mousedown','3')
 poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'held recipient ledger before source root resize')
 live_capture('primary',displays[0],'forwarding','-before-source-resize');live_capture('recipient',displays[1],'receiving','-before-source-resize')
 try:
  subprocess.run(['xrandr','--output','screen','--off','--fb','800x600'],env=env|{'DISPLAY':displays[0]},check=True)
  primary.wait(timeout=5);recipient.wait(timeout=5)
  if primary.returncode==0 or 'source geometry changed' not in (out/'primary.log').read_text():raise AssertionError('source resize did not preserve geometry failure')
  poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'source resize joins recipient held key/right-button release')
  command(displays[0],'keyup','a','mouseup','3')
  d=display_handles[0];root=x.XDefaultRootWindow(d)
  if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('source resize retained source keyboard')
  x.XUngrabKeyboard(d,0)
  if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('source resize retained source pointer')
  x.XUngrabPointer(d,0);x.XSync(d,0)
  if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('source resize retained status window')
  subprocess.run(['import','-display',displays[0],'-window','root',str(out/'actual-launcher-source-resize.png')],check=True)
  assert Image.open(out/'actual-launcher-source-resize.png').size==(800,600),'actual launcher source root did not shrink'
  address=fresh['address'].rsplit(':',1)
  with socket.socket() as probe:
   probe.settimeout(.2)
   if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('source resize left recipient listener open')
 finally:
  subprocess.run(['xrandr','--fb','1024x768','--output','screen','--mode','1024x768'],env=env|{'DISPLAY':displays[0]},check=True)
 command(displays[0],'mousemove','500','300','mousemove','1023','300');time.sleep(.05)
 if primary.poll() is None or recipient.poll() is None:raise AssertionError('restored root reactivated old launcher')
 print('PASS actual source root resize ends both launchers, preserves geometry error, releases recipient key/right-button, reclaims source and closes status/listener; root restoration cannot reactivate run',flush=True)
 # Recipient root loss must independently end its native injector and both
 # launchers while key/right-button remain held, without a source key-up.
 for role in ('primary','recipient'):
  fresh_path=out/role/'run.json';fresh=json.loads(fresh_path.read_text());fresh['binding']['run']='1'*64;fresh_path.write_text(json.dumps(fresh))
 recipient=launch('recipient');approve('recipient',displays[1],name='-recipient-resize')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'recipient resize listening')
 primary=launch('primary');approve('primary',displays[0],name='-recipient-resize')
 command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
 poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'recipient resize active')
 command(displays[0],'keydown','a','mousedown','3')
 poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'held native ledger before recipient resize')
 try:
  subprocess.run(['xrandr','--output','screen','--off','--fb','800x600'],env=env|{'DISPLAY':displays[1]},check=True)
  primary.wait(timeout=5);recipient.wait(timeout=5)
  poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'recipient resize releases native held ledger without source keyup')
  if primary.returncode==0 or recipient.returncode==0:raise AssertionError('recipient resize incorrectly reported clean success')
  command(displays[0],'keyup','a','mouseup','3')
  d=display_handles[0];root=x.XDefaultRootWindow(d)
  if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('recipient resize retained source keyboard')
  x.XUngrabKeyboard(d,0)
  if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('recipient resize retained source pointer')
  x.XUngrabPointer(d,0);x.XSync(d,0)
  if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('recipient resize left status window')
  subprocess.run(['import','-display',displays[1],'-window','root',str(out/'actual-launcher-recipient-resize.png')],check=True)
  assert Image.open(out/'actual-launcher-recipient-resize.png').size==(800,600),'actual recipient root did not shrink'
  address=fresh['address'].rsplit(':',1)
  with socket.socket() as probe:
   probe.settimeout(.2)
   if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('recipient resize left listener open')
 finally:
  original=size.rsplit('x',1)[0]
  subprocess.run(['xrandr','--fb',original,'--output','screen','--mode',original],env=env|{'DISPLAY':displays[1]},check=True)
 command(displays[0],'mousemove','500','300','mousemove','1023','300');time.sleep(.05)
 if primary.poll() is None or recipient.poll() is None:raise AssertionError('recipient root restoration reactivated run')
 print('PASS actual recipient root resize releases native held key/right-button without source keyup, ends both launchers with errors, reclaims source and closes status/listener; no reactivation',flush=True)
 # Keyboard-map retirement is checked through both actual launchers too,
 # not inferred from the fixture-only capture/recipient checks. Restore only
 # after the native ledger has been observed released, never as recovery.
 for changed_role,run_id in [('primary','0'*64),('recipient','ab'*32)]:
  for role in ('primary','recipient'):
   fresh_path=out/role/'run.json';fresh=json.loads(fresh_path.read_text());fresh['binding']['run']=run_id;fresh_path.write_text(json.dumps(fresh))
  recipient=launch('recipient');approve('recipient',displays[1],name='-map-'+changed_role)
  poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'map retirement recipient listen')
  primary=launch('primary');approve('primary',displays[0],name='-map-'+changed_role)
  command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
  poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'map retirement active')
  command(displays[0],'keydown','a','mousedown','3')
  poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'native key/right-button held before '+changed_role+' map change')
  restore=change_keymap(display_handles[0 if changed_role=='primary' else 1])
  try:
   primary.wait(timeout=5);recipient.wait(timeout=5)
   poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'map loss native release before map restoration/source keyup')
   if primary.returncode==0:raise AssertionError('map retirement reported clean success at source')
   if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('map retirement left live status')
   d=display_handles[0];root=x.XDefaultRootWindow(d)
   if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('map retirement retained source keyboard')
   x.XUngrabKeyboard(d,0)
   if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('map retirement retained source pointer')
   x.XUngrabPointer(d,0);x.XSync(d,0)
   address=fresh['address'].rsplit(':',1)
   with socket.socket() as probe:
    probe.settimeout(.2)
    if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('map retirement retained listener')
  finally:restore()
  command(displays[0],'keyup','a','mouseup','3','mousemove','500','300','mousemove','1023','300');time.sleep(.05)
  if primary.poll() is None or recipient.poll() is None:raise AssertionError('restored keyboard map reactivated retired run')
  print('PASS actual '+changed_role+' keyboard-map change retires both launchers, releases recipient key/right-button before map restoration/source keyup, reclaims source and closes status/listener; no reactivation',flush=True)
 # Approved waiting recipient must retire even before any peer/injector.
 for waiting_mode,run_id in [('resize','cd'*32),('transient','ef'*32)]:
  path=out/'recipient'/'run.json';fresh=json.loads(path.read_text());fresh['binding']['run']=run_id;path.write_text(json.dumps(fresh))
  waiting=launch('recipient');approve('recipient',displays[1],name='-waiting-root-'+waiting_mode)
  poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'waiting geometry recipient listener')
  live_capture('recipient',displays[1],'waiting','-before-root-'+waiting_mode)
  child_pids=set()
  for children_file in (pathlib.Path('/proc')/str(waiting.pid)/'task').glob('*/children'):
   try:child_pids.update(children_file.read_text().split())
   except FileNotFoundError:pass
  for pid in child_pids:
   try:
    if b'--mnet-private-desk-input-v1' in (pathlib.Path('/proc')/pid/'cmdline').read_bytes().split(b'\0'):raise AssertionError('waiting run already created native input child')
   except FileNotFoundError:pass
  original=size.rsplit('x',1)[0]
  try:
   subprocess.run(['xrandr','--output','screen','--off','--fb','800x600'],env=env|{'DISPLAY':displays[1]},check=True)
   if waiting_mode=='transient':subprocess.run(['xrandr','--fb',original,'--output','screen','--mode',original],env=env|{'DISPLAY':displays[1]},check=True)
   waiting.wait(timeout=3)
   if waiting.returncode==0 or 'reviewed recipient geometry changed' not in (out/'recipient.log').read_text():raise AssertionError('waiting root loss did not preserve geometry error')
   if live_window('recipient',displays[1]):raise AssertionError('waiting root change retained local status')
   address=fresh['address'].rsplit(':',1)
   with socket.socket() as probe:
    probe.settimeout(.2)
    if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('waiting root change retained approved listener')
   if waiting_mode=='resize':
    subprocess.run(['import','-display',displays[1],'-window','root',str(out/'waiting-recipient-geometry-retired.png')],check=True)
    assert Image.open(out/'waiting-recipient-geometry-retired.png').size==(800,600)
  finally:subprocess.run(['xrandr','--fb',original,'--output','screen','--mode',original],env=env|{'DISPLAY':displays[1]},check=True)
  if waiting.poll() is None:raise AssertionError('restored geometry reactivated waiting run')
  replay=subprocess.run([str(out/'mnetd'),'-desk-run',str(path)],env=env|{'DISPLAY':displays[1]},capture_output=True,text=True,timeout=3)
  if replay.returncode==0 or 'already consumed' not in replay.stderr:raise AssertionError('geometry-loss run replay allowed')
  print('PASS actual approved waiting-recipient root '+waiting_mode+' retires status/listener before any peer/native input, preserves geometry error and consumed run; restoration cannot resume',flush=True)
 # Abruptly losing either actual launcher must retire its counterpart and
 # release recipient-owned transitions. This does not kill the native child;
 # that surviving child's own lease/EOF cleanup is exactly what is checked.
 for lost,token in [('primary','4'),('recipient','3')]:
  for role in ('primary','recipient'):
   fresh_path=out/role/'run.json';fresh=json.loads(fresh_path.read_text());fresh['binding']['run']=token*64;fresh_path.write_text(json.dumps(fresh))
  recipient=launch('recipient');approve('recipient',displays[1],name='-crash-'+lost)
  poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'crash recipient listening')
  primary=launch('primary');approve('primary',displays[0],name='-crash-'+lost)
  command(displays[0],'mousemove','500','300');time.sleep(.05);command(displays[0],'mousemove','1023','300')
  poll(lambda:'sending raw input' in (out/'primary.log').read_text(),'crash source active')
  command(displays[0],'keydown','a','mousedown','3')
  poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'native held input before launcher crash')
  child_pids=set()
  for children_file in (pathlib.Path('/proc')/str(recipient.pid)/'task').glob('*/children'):
   try:child_pids.update(children_file.read_text().split())
   except FileNotFoundError:pass
  input_children=[int(pid) for pid in child_pids if b'--mnet-private-desk-input-v1' in (pathlib.Path('/proc')/pid/'cmdline').read_bytes().split(b'\0')]
  if len(input_children)!=1:raise AssertionError('exact native input child not found before launcher crash')
  native_children.update(input_children)
  victim=primary if lost=='primary' else recipient;survivor=recipient if lost=='primary' else primary
  victim.kill();victim.wait(timeout=5);survivor.wait(timeout=5)
  poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'native release after '+lost+' launcher SIGKILL')
  def child_ended():
   try:return (pathlib.Path('/proc')/str(input_children[0])/'stat').read_text().rsplit(')',1)[1].strip().split()[0]=='Z'
   except FileNotFoundError:return True
  poll(child_ended,'native child cleanup exit after '+lost+' launcher crash')
  try:os.waitpid(input_children[0],0)
  except ChildProcessError:pass # The live recipient already joined its own child.
  if survivor.returncode==0:raise AssertionError('abrupt peer loss reported clean success')
  command(displays[0],'keyup','a','mouseup','3')
  d=display_handles[0];root=x.XDefaultRootWindow(d)
  if x.XGrabKeyboard(d,root,0,1,1,0)!=0:raise AssertionError('launcher crash retained source keyboard')
  x.XUngrabKeyboard(d,0)
  if x.XGrabPointer(d,root,0,0,1,1,0,0,0)!=0:raise AssertionError('launcher crash retained source pointer')
  x.XUngrabPointer(d,0);x.XSync(d,0)
  if live_window('primary',displays[0]) or live_window('recipient',displays[1]):raise AssertionError('launcher crash retained active status')
  print('PASS actual '+lost+' launcher SIGKILL releases recipient key/right-button, reclaims source and reports peer loss as error; native child survives to clean up',flush=True)
 # An authenticated exact-key peer with the exact approved hello can still
 # send invalid input. Refusal must release the already-held native ledger
 # before this fixture closes TLS, not merely on socket EOF or source keyup.
 for malformed,token in [('duplicate-key','e5'),('stale-sequence','e6'),('unknown-kind','e7'),('missing-heartbeat','e8'),('partial-frame','e9')]:
  recipient_path=out/'recipient'/'run.json';bad_spec=json.loads(recipient_path.read_text());bad_spec['binding']['run']=token*32;bad_spec['lifetime_seconds']=30;recipient_path.write_text(json.dumps(bad_spec))
  recipient=launch('recipient');approve('recipient',displays[1],name='-invalid-'+malformed)
  poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'invalid input recipient listening')
  context=ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT);context.check_hostname=False;context.verify_mode=ssl.CERT_NONE
  context.minimum_version=ssl.TLSVersion.TLSv1_3;context.set_alpn_protocols(['mnet-desk-1']);context.load_cert_chain(str(out/'primary'/'cert.pem'),str(out/'primary'/'key.pem'))
  address=bad_spec['address'].rsplit(':',1)
  raw=context.wrap_socket(socket.create_connection((address[0],int(address[1])),timeout=2),server_hostname=None)
  def send_frame(message):
   encoded=json.dumps(message).encode();raw.sendall(len(encoded).to_bytes(4,'big')+encoded)
  def receive_exact(length):
   result=b''
   while len(result)<length:
    chunk=raw.recv(length-len(result))
    if not chunk:raise AssertionError('TLS ended before ready')
    result+=chunk
   return result
  try:
   send_frame(dict(kind='hello',binding=bad_spec['binding']))
   count=int.from_bytes(receive_exact(4),'big');assert 0<count<=4096
   assert json.loads(receive_exact(count))==dict(kind='ready',ok=True),'no exact ready'
   send_frame(dict(kind='input',input=dict(sequence=1,kind='key',action='down',code=38)))
   send_frame(dict(kind='input',input=dict(sequence=2,kind='button',action='down',code=3)))
   poll(lambda:keysbuttons(display_handles[1],1024)==(True,True),'native ledger before '+malformed)
   live_capture('recipient',displays[1],'receiving','-invalid-'+malformed)
   if malformed=='duplicate-key':message=dict(kind='input',input=dict(sequence=3,kind='key',action='down',code=38))
   elif malformed=='stale-sequence':message=dict(kind='input',input=dict(sequence=2,kind='pointer',action='move',x=10,y=10))
   else:message=dict(kind='clipboard')
   if malformed=='partial-frame':raw.sendall((4096).to_bytes(4,'big')+b'{')
   elif malformed!='missing-heartbeat':send_frame(message)
   recipient.wait(timeout=4)
   assert recipient.returncode!=0,'invalid authenticated input reported success'
   reason={'duplicate-key':'duplicate press','stale-sequence':'stale input sequence','unknown-kind':'unsupported desk message','missing-heartbeat':'desk channel interrupted','partial-frame':'desk channel interrupted'}[malformed]
   assert reason in (out/'recipient.log').read_text(),'invalid input ended for an unrelated reason'

   poll(lambda:keysbuttons(display_handles[1],1024)==(False,False),'native release before TLS close after '+malformed)
   assert not live_window('recipient',displays[1]),'invalid input retained status'
   with socket.socket() as probe:
    probe.settimeout(.2)
    assert probe.connect_ex((address[0],int(address[1])))!=0,'invalid input kept listener'
   assert (out/'recipient'/'runs'/(token*32)).exists(),'invalid input lost consumed token'
  finally:raw.close()
  replay=subprocess.run([str(out/'mnetd'),'-desk-run',str(recipient_path)],env=env|{'DISPLAY':displays[1]},capture_output=True,text=True,timeout=3)
  assert replay.returncode!=0 and 'already consumed' in replay.stderr,'invalid-input run replay was not refused'
  assert not live_window('recipient',displays[1]),'invalid-input replay retained status'
  print('PASS actual exact-key/exact-run '+malformed+' retires native held key/right-button before client TLS close, closes status/listener and retains consumed run',flush=True)
 # First-connection failures consume approval, not an invitation to retry.
 # All certificates/keys are generated only for these disposable processes.
 for mode,token in (('plaintext','d1'),('wrong-role-certificate','d2'),('pending-tls-stop','d3'),('exact-key-wrong-run','d4')):
  path=out/'recipient'/'run.json';fresh=json.loads(path.read_text());fresh['binding']['run']=token*32;fresh['lifetime_seconds']=30;path.write_text(json.dumps(fresh))
  waiting=launch('recipient');approve('recipient',displays[1],name='-'+mode)
  poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),mode+' recipient listener')
  before_children=(pathlib.Path('/proc')/str(waiting.pid)/'task'/str(waiting.pid)/'children').read_text().strip()
  if before_children:raise AssertionError('waiting untrusted peer created native child')
  raw=socket.create_connection((address[0],int(address[1])),timeout=1)
  try:
   if mode=='plaintext':
    raw.sendall(b'not-a-TLS-client');raw.shutdown(socket.SHUT_WR)
   elif mode in ('wrong-role-certificate','exact-key-wrong-run'):
    context=ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT);context.check_hostname=False;context.verify_mode=ssl.CERT_NONE
    context.minimum_version=ssl.TLSVersion.TLSv1_3;context.set_alpn_protocols(['mnet-desk-1'])
    identity='recipient' if mode=='wrong-role-certificate' else 'primary'
    context.load_cert_chain(str(out/identity/'cert.pem'),str(out/identity/'key.pem'))
    raw=context.wrap_socket(raw,server_hostname=None)
    if mode=='exact-key-wrong-run':
     # Possession of the expected primary key is not the exact local grant.
     wrong=dict(fresh['binding'],run='0'*64);body=json.dumps(dict(kind='hello',binding=wrong)).encode();raw.sendall(len(body).to_bytes(4,'big')+body)
    try:raw.recv(1)
    except (ssl.SSLError,ConnectionResetError):pass
   else:
    # An accepted TCP socket blocked before ClientHello must remain locally
    # stoppable without waiting for its TLS timeout or starting an injector.
    time.sleep(.1)
    if waiting.poll() is not None:raise AssertionError('pending TLS retired before local stop check')
    if (pathlib.Path('/proc')/str(waiting.pid)/'task'/str(waiting.pid)/'children').read_text().strip():raise AssertionError('pending TLS created native child')
    if keysbuttons(display_handles[1],1024)!=(False,False):raise AssertionError('untrusted socket changed recipient input')
    wid=live_capture('recipient',displays[1],'waiting','-'+mode)
    command(displays[1],'windowfocus',wid,'mousemove','--window',wid,'850','290','click','1')
   waiting.wait(timeout=4)
  finally:raw.close()
  if waiting.returncode==0:raise AssertionError('first connection failure became success')
  diagnostic=(out/'recipient.log').read_text()
  if mode=='wrong-role-certificate' and 'TLS peer or protocol differs' not in diagnostic:raise AssertionError('wrong-role exact-key refusal missing')
  if mode=='exact-key-wrong-run' and 'exact local approval mismatch' not in diagnostic:raise AssertionError('exact primary key bypassed run binding')
  if keysbuttons(display_handles[1],1024)!=(False,False):raise AssertionError('refused peer left native input held')
  if mode=='plaintext' and 'TLS handshake failed' not in diagnostic:raise AssertionError('plaintext TLS refusal missing')
  if live_window('recipient',displays[1]):raise AssertionError('untrusted connection left status active')
  with socket.socket() as probe:
   probe.settimeout(.2)
   if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('untrusted first connection kept listener for retry')
  replay=subprocess.run([str(out/'mnetd'),'-desk-run',str(path)],env=env|{'DISPLAY':displays[1]},capture_output=True,text=True,timeout=3)
  if replay.returncode==0 or 'already consumed' not in replay.stderr:raise AssertionError('first connection failure renewed approval')
  print('PASS actual first-connection '+mode+' retires listener/status with failure and consumed run; no retry or input authority',flush=True)
 # The deadline includes local review and waiting. Check the actual listener
 # and process both end rather than leaving a dormant approved service.
 spec['binding']['run']='b'*64;spec['lifetime_seconds']=2;path=out/'recipient'/'run.json';path.write_text(json.dumps(spec))
 timed=launch('recipient');approve('recipient',displays[1],name='-deadline')
 poll(lambda:'waiting for one exact-key' in (out/'recipient.log').read_text(),'deadline recipient listen')
 timed.wait(timeout=5)
 if timed.returncode==0 or 'canceled or accept failed' not in (out/'recipient.log').read_text():raise AssertionError('waiting recipient deadline not terminal')
 with socket.socket() as probe:
  probe.settimeout(.2)
  if probe.connect_ex((address[0],int(address[1])))==0:raise AssertionError('deadline left listener open')
 if live_window('recipient',displays[1]):raise AssertionError('deadline left status window open')
 print('PASS actual run deadline retires waiting recipient and closes listener/window',flush=True)

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
