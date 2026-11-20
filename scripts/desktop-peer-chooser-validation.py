#!/usr/bin/env python3
"""Actual native chooser, private synthetic local API; no live peer discovery."""
import base64,http.server,json,os,pathlib,secrets,socket,socketserver,subprocess,tempfile,threading,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-peer-chooser.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
state=out/'state';state.mkdir(mode=0o700);token=secrets.token_hex(32);(state/'local-api-token').write_text(token);(state/'local-api-token').chmod(0o600)
peers=[dict(name=f'fixture-peer-{i:02}',ipv4=f'127.0.0.{i+1}',PublicKey=base64.b64encode(bytes([i])*32).decode()) for i in range(13)]
class Server(socketserver.ThreadingMixIn,socketserver.UnixStreamServer):daemon_threads=True
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self,*_):pass
 def do_POST(self):self.server.bad.append('write');self.send_response(403);self.end_headers()
 def do_GET(self):
  self.server.calls.append(self.path);self.server.call_times.append(time.monotonic())
  if self.headers.get('X-Mnet-Local-Token')!=token or self.path not in ['/localapi/status','/localapi/session-peers']:
   self.server.bad.append('scope');self.send_response(403);self.end_headers();return
  time.sleep(self.server.delay)
  data=(dict(status=self.server.status,node_name='fixture-viewer',peers=[dict(name=p['name'],ipv4=p['ipv4']) for p in self.server.peers]) if self.path=='/localapi/status' else self.server.peers)
  raw=b'invalid-private-response' if self.server.malformed else json.dumps(data).encode()
  try:self.send_response(200);self.end_headers();self.wfile.write(raw)
  except BrokenPipeError:pass
server=Server(str(state/'mnetd.sock'),Handler);server.calls=[];server.call_times=[];server.bad=[];server.delay=0;server.peers=peers;server.status='up';server.malformed=False
thread=threading.Thread(target=server.serve_forever);thread.start();x=None;client=None;env=os.environ.copy();log=None
def poll(fn,label,seconds=5):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  value=fn()
  if value:return value
  if client is not None and client.poll() is not None:raise AssertionError(label+': '+(out/'client.log').read_text())
  time.sleep(.02)
 raise AssertionError('deadline '+label)
def action(*args):subprocess.run(['xdotool',*args],env=env,check=True)
def window():
 r=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet Remote Desktop: choose'],env=env,capture_output=True,text=True)
 return r.stdout.strip().splitlines()[-1] if r.returncode==0 and r.stdout.strip() else None
def title():return subprocess.check_output(['xdotool','getwindowname',wid],env=env,text=True).strip()
def phase(name,read=None):poll(lambda:'['+name+']' in title() and (read is None or 'read'+str(read)+' ' in title()),name)
def capture(name):
 p=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(p)],check=True);im=Image.open(p).convert('RGB');assert im.size==(900,560)
 return im
try:
 with (out/'display').open('w') as fd,(out/'x.log').open('w') as xl:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=xl,stderr=xl)
 env['DISPLAY']=':'+poll(lambda:(out/'display').read_text().strip(),'display');env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 def launch():
  global client,wid
  with (out/'client.log').open('w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'session','browse'],env=env,stdout=log,stderr=log)
  wid=poll(window,'chooser');action('windowfocus',wid)
 launch();phase('ready',1);capture('ready-no-selection');assert len(server.calls)==2
 # Neither row nor mode release may borrow an unrelated mouse press.
 action('mousemove','--window',wid,'30','80','mousedown','1','mousemove','--window',wid,'100','120','mouseup','1')
 capture('outside-row-no-selection');assert len(server.calls)==2
 action('mousemove','--window',wid,'100','120','click','1');capture('selected')
 action('mousemove','--window',wid,'100','120','mousedown','1','mousemove','--window',wid,'620','510','mouseup','1')
 time.sleep(.05);assert client.poll() is None and len(server.calls)==2,'row press became pointer choice'
 focus_app=subprocess.Popen(['xmessage','-title','Chooser focus fixture','-buttons','OK','Local focus fixture'],env=env,stdout=(out/'focus.log').open('w'),stderr=subprocess.PIPE)
 try:
  def other_window():
   r=subprocess.run(['xdotool','search','--onlyvisible','--name','^Chooser focus fixture$'],env=env,capture_output=True,text=True)
   return r.stdout.strip().splitlines()[-1] if r.returncode==0 and r.stdout.strip() else None
  other=poll(other_window,'independent focus window')
  action('windowfocus',wid,'mousemove','--window',wid,'620','510','mousedown','1','windowfocus',other,'mouseup','1','windowfocus',wid)
  time.sleep(.05);assert client.poll() is None and len(server.calls)==2,'focus loss retained Pointer view gesture'
 finally:focus_app.terminate();focus_app.communicate(timeout=2)
 # Home/End select exact endpoints across pages, never a mode connection.
 action('key','End');poll(lambda:title().endswith('page3'),'End last page');im=capture('keyboard-last-peer')
 assert im.getpixel((25,105))==(0,102,204),'End did not select last visible peer'
 assert len(server.calls)==2 and client.poll() is None,'End fetched or connected'
 action('key','Home');poll(lambda:title().endswith('page1'),'Home first page');im=capture('keyboard-first-peer')
 assert im.getpixel((25,105))==(0,102,204),'Home did not select first visible peer'
 assert len(server.calls)==2 and client.poll() is None,'Home fetched or connected'
 # Restore the original keyboard journey's starting focus and next-row step.
 action('windowfocus',wid)
 action('key','Down')
 for _ in range(7):action('key','Tab')
 poll(lambda:capture('keyboard-view-focus').getpixel((413,493))==(255,255,255),'actual View focus pixels')
 action('keydown','Return','key','Tab','keyup','Return')
 time.sleep(.05);assert client.poll() is None and len(server.calls)==2,'Tab moved pending View key to Pointer choice'
 poll(lambda:capture('keyboard-pointer-focus').getpixel((563,493))==(255,255,255),'actual Pointer focus pixels')
 action('keyup','space');time.sleep(.05);assert client.poll() is None and len(server.calls)==2,'unpressed mode release connected'
 action('mousemove','--window',wid,'330','510','click','1');poll(lambda:title().endswith('page2'),'second page')
 action('click','1');poll(lambda:title().endswith('page3'),'third page');capture('selected-off-page');assert len(server.calls)==2,'pagination fetched daemon'
 # Refresh clears old selection, even when the exact rows recur.
 action('key','r');phase('ready',2);capture('refreshed-selection-cleared');assert len(server.calls)==4
 action('mousemove','--window',wid,'620','510','click','1');time.sleep(.05);assert client.poll() is None and len(server.calls)==4,'refresh retained selected peer'
 # Blank/malformed responses cannot keep prior rows or input actions.
 server.peers=[];action('key','r');phase('ready',3);capture('empty')
 server.malformed=True;action('key','r');phase('unavailable',4);capture('unavailable')
 # Each successful read is delayed1s, so receipt-based age would extend the
 # two-request snapshot by2s. Expire by30.8s from the first observed request,
 # before30s have elapsed since its final response. This uses the real window.
 server.malformed=False;server.peers=peers;server.delay=1;first=len(server.calls)
 action('key','r');phase('ready',5);received=time.monotonic();server.delay=0
 started=server.call_times[first];assert received-started>=2,'delayed evidence fixture did not wait'
 action('key','Down');capture('keyboard-selected')
 calls=len(server.calls);time.sleep(max(0,started+30.8-time.monotonic()))
 assert '[expired]' in title(),'snapshot age began after final authorization response'
 assert time.monotonic()<received+30,'expiry observation did not distinguish request/receipt ages'
 print('PASS actual delayed chooser expiry: request age %.2fs, receipt age %.2fs, no automatic read'%(time.monotonic()-started,time.monotonic()-received),flush=True)
 capture('expired');assert len(server.calls)==calls,'chooser automatically polled'
 action('mousemove','--window',wid,'620','510','click','1');time.sleep(.05);assert client.poll() is None and len(server.calls)==calls,'expired chooser connected'
 # A selected row cannot silently become a replacement, missing, duplicate,
 # renamed, moved or non-Up peer while its mode gesture opens the session.
 # Each case starts from a fresh real chooser and owns a passive TCP sentinel.
 action('key','Escape');assert client.wait(timeout=3)==0;client=None
 for change in ('key','missing','name','address','duplicate','down'):
  server.peers=peers;server.status='up';launch();phase('ready',1);action('key','Down')
  if change=='key':server.peers=[dict(p,PublicKey=base64.b64encode(b'z'*32).decode()) for p in peers]
  elif change=='missing':server.peers=peers[1:]
  elif change=='name':server.peers=[dict(p,name='renamed-'+p['name']) for p in peers]
  elif change=='address':server.peers=[dict(p,ipv4='127.0.1.'+str(i+1)) for i,p in enumerate(peers)]
  elif change=='duplicate':server.peers=[peers[0],*peers]
  else:server.status='down'
  with socket.socket() as listener:
   listener.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);listener.bind(('127.0.0.1',5909));listener.listen();listener.settimeout(.2)
   action('mousemove','--window',wid,'470','510','click','1');assert client.wait(timeout=6)!=0;client=None
   try:conn,_=listener.accept();conn.close();raise AssertionError(change+' peer reached TCP')
   except TimeoutError:pass
  if change in ('key','name'):assert 'selected peer changed' in (out/'client.log').read_text(),'missing exact-selection refusal'
  print('PASS actual selected peer '+change+' change refused before TCP',flush=True)
 server.status='up'
 server.peers=peers;server.delay=2;launch();phase('loading');action('key','Escape');assert client.wait(timeout=2)==0;client=None
 assert not server.bad
 print('PASS actual chooser:13 exact peers/three local pages, Home/End endpoints without reads/connections, explicit row+mode, mismatched gestures refused, refresh/empty/error/expiry clears selection, no polling, replaced/missing/renamed/moved/duplicate/non-Up peer refused before TCP, close cancels local read; synthetic local evidence only',flush=True)
finally:
 if client is not None and client.poll() is None:client.terminate();client.wait(timeout=3)
 server.shutdown();server.server_close();thread.join(timeout=3);(state/'local-api-token').unlink(missing_ok=True)
 if x is not None:x.terminate();x.wait(timeout=3)
