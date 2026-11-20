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
  self.server.calls.append(self.path)
  if self.headers.get('X-Mnet-Local-Token')!=token or self.path not in ['/localapi/status','/localapi/session-peers']:
   self.server.bad.append('scope');self.send_response(403);self.end_headers();return
  time.sleep(self.server.delay)
  data=(dict(status=self.server.status,node_name='fixture-viewer',peers=[dict(name=p['name'],ipv4=p['ipv4']) for p in self.server.peers]) if self.path=='/localapi/status' else self.server.peers)
  raw=b'invalid-private-response' if self.server.malformed else json.dumps(data).encode()
  try:self.send_response(200);self.end_headers();self.wfile.write(raw)
  except BrokenPipeError:pass
server=Server(str(state/'mnetd.sock'),Handler);server.calls=[];server.bad=[];server.delay=0;server.peers=peers;server.status='up';server.malformed=False
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
 action('key','Down')
 for _ in range(7):action('key','Tab')
 poll(lambda:capture('keyboard-view-focus').getpixel((413,493))==(255,255,255),'actual View focus pixels')
 action('keydown','Return','key','Tab','keyup','Return')
 time.sleep(.05);assert client.poll() is None and len(server.calls)==2,'Tab moved pending View key to Pointer choice'
 poll(lambda:capture('keyboard-pointer-focus').getpixel((563,493))==(255,255,255),'actual Pointer focus pixels')
 action('keyup','space');time.sleep(.05);assert client.poll() is None and len(server.calls)==2,'unpressed mode release connected'
 action('mousemove','--window',wid,'330','510','click','1');poll(lambda:title().endswith('page2'),'second page')
 action('click','1');poll(lambda:title().endswith('page3'),'third page');assert len(server.calls)==2,'pagination fetched daemon'
 # Refresh clears old selection, even when the exact rows recur.
 action('key','r');phase('ready',2);capture('refreshed-selection-cleared');assert len(server.calls)==4
 action('mousemove','--window',wid,'620','510','click','1');time.sleep(.05);assert client.poll() is None and len(server.calls)==4,'refresh retained selected peer'
 # Blank/malformed responses cannot keep prior rows or input actions.
 server.peers=[];action('key','r');phase('ready',3);capture('empty')
 server.malformed=True;action('key','r');phase('unavailable',4);capture('unavailable')
 server.malformed=False;server.peers=peers;action('key','r');phase('ready',5);action('key','Down');capture('keyboard-selected')
 calls=len(server.calls);time.sleep(31);phase('expired');capture('expired');assert len(server.calls)==calls,'chooser automatically polled'
 action('mousemove','--window',wid,'620','510','click','1');time.sleep(.05);assert client.poll() is None and len(server.calls)==calls,'expired chooser connected'
 # Exact selected-key replacement must fail before the TCP session listener.
 action('key','r');phase('ready',6);action('key','Down')
 server.peers=[dict(p,PublicKey=base64.b64encode(b'z'*32).decode()) for p in peers]
 with socket.socket() as listener:
  listener.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);listener.bind(('127.0.0.1',5909));listener.listen();listener.settimeout(.2)
  action('mousemove','--window',wid,'470','510','click','1');assert client.wait(timeout=6)!=0;client=None
  try:conn,_=listener.accept();conn.close();raise AssertionError('replaced peer reached TCP')
  except TimeoutError:pass
 assert 'selected peer changed' in (out/'client.log').read_text(),'missing exact-key refusal'
 server.peers=peers;server.delay=2;launch();phase('loading');action('key','Escape');assert client.wait(timeout=2)==0;client=None
 assert not server.bad
 print('PASS actual chooser:13 exact peers/three local pages, explicit row+mode, mismatched gestures refused, refresh/empty/error/expiry clears selection, no polling, replaced key refused before TCP, close cancels local read; synthetic local evidence only',flush=True)
finally:
 if client is not None and client.poll() is None:client.terminate();client.wait(timeout=3)
 server.shutdown();server.server_close();thread.join(timeout=3);(state/'local-api-token').unlink(missing_ok=True)
 if x is not None:x.terminate();x.wait(timeout=3)
