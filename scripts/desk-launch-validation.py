#!/usr/bin/env python3
"""Actual one-run engineering CLI processes on two disposable X servers."""
import ctypes,ctypes.util,json,os,pathlib,shutil,signal,socket,subprocess,tempfile,time
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for dep in ('go','Xvfb','xdotool','import','zenity'):
 if not shutil.which(dep):raise SystemExit('missing dependency: '+dep)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-launch.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnetd'),'./cmd/mnetd'],check=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'fixture.test'),'./cmd/mnetd'],check=True)
xs=[];children=[];display_handles=[]
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
def command(display,*args):subprocess.run(['xdotool',*args],env=env|{'DISPLAY':display},check=True)
def approve(role,display,action='alt+a',name=''):
 title='^Mnet Shared Desk: review '+role+'$'
 def find():
  result=subprocess.run(['xdotool','search','--onlyvisible','--name',title],env=env|{'DISPLAY':display},capture_output=True,text=True)
  return result.stdout.strip().splitlines()[-1] if result.returncode==0 and result.stdout.strip() else None
 wid=poll(find,role+' review visible')
 subprocess.run(['import','-display',display,'-window',wid,str(out/('launch-review-'+role+name+'.png'))],check=True)
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
 command(displays[1],'mousemove','--window',stop_window,'360','267','click','1');waiting.wait(timeout=5)
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
 # The deadline includes local review and waiting. Check the actual listener
 # and process both end rather than leaving a dormant approved service.
 spec['binding']['run']='b'*64;spec['lifetime_seconds']=2;path.write_text(json.dumps(spec))
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
 for d in display_handles:x.XCloseDisplay(d)
 # Never retain disposable TLS private keys in a CI artifact.
 for role in ('primary','recipient'):(out/role/'key.pem').unlink(missing_ok=True)
 for p in xs:p.terminate();p.wait()
