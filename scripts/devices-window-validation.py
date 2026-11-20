#!/usr/bin/env python3
"""Actual read-only built Mnet window against a private Unix daemon fixture."""
import http.server,json,os,pathlib,secrets,shutil,socketserver,subprocess,tempfile,threading,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for dependency in ('go','Xvfb','xdotool','import','xmessage'):
 if not shutil.which(dependency):raise SystemExit('missing dependency: '+dependency)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-devices-window.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
state=out/'state';state.mkdir(mode=0o700)
token=secrets.token_hex(32);token_file=state/'local-api-token';token_file.write_text(token);token_file.chmod(0o600)
class Server(socketserver.ThreadingMixIn,socketserver.UnixStreamServer):daemon_threads=True
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self,*_):pass
 def do_POST(self):
  self.server.bad.append(self.command);self.send_response(403);self.end_headers()
 def do_GET(self):
  self.server.requests.append((self.path,self.headers.get('X-Mnet-Local-Token')))
  if self.path!='/localapi/status' or self.headers.get('X-Mnet-Local-Token')!=token:
   self.server.bad.append(self.path);self.send_response(403);self.end_headers();return
  payload=self.server.payload;code=self.server.code;delay=self.server.delay
  self.server.entered.set();time.sleep(delay)
  data=json.dumps(payload).encode() if not isinstance(payload,bytes) else payload
  try:
   self.send_response(code);self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
  except (BrokenPipeError,ConnectionResetError):pass
server=Server(str(state/'mnetd.sock'),Handler);os.chmod(state/'mnetd.sock',0o600)
server.bad=[];server.requests=[];server.delay=0;server.code=200;server.entered=threading.Event()
peers=[]
for i in range(19):
 handshake=['unavailable','never','clock-skew','recent','stale'][i%5]
 peers.append(dict(name=f'synthetic-peer-{i:02}',ipv4=f'100.64.0.{i+2}',online=handshake=='recent',handshake_state=handshake,last_handshake_ago_sec=0 if handshake=='recent' else -1,endpoint='PRIVATE_ENDPOINT' if i%2 else '',relay=i%3==0))
up=dict(status='up',node_name='Synthetic local device',network='Private fixture only',ipv4='100.64.0.1',peers=peers,approval_url='PRIVATE_APPROVAL_URL',server='PRIVATE_SERVER_URL',public_key='PRIVATE_KEY',last_error='PRIVATE_ERROR')
server.payload=up
thread=threading.Thread(target=server.serve_forever);thread.start()
x=None;client=None;focus_fixture=None;env=os.environ.copy()
def poll(fn,label,seconds=6):
 until=time.monotonic()+seconds
 while time.monotonic()<until:
  result=fn()
  if result:return result
  if client is not None and client.poll() is not None:raise AssertionError('window exited before '+label+': '+(out/'client.log').read_text())
  time.sleep(.02)
 raise AssertionError('deadline: '+label)
def action(*args):subprocess.run(['xdotool',*args],env=env,check=True)
def title():return subprocess.check_output(['xdotool','getwindowname',wid],env=env,text=True).strip()
def phase(name,read=None):poll(lambda:'['+name+']' in title() and (read is None or 'read'+str(read)+' ' in title()),name)
def refresh(name,read):action('key','r');phase(name,read)
def capture(name):
 path=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(path)],check=True)
 image=Image.open(path).convert('RGB');assert image.size==(900,500)
 assert len(image.getcolors(900*500) or [])>3,'empty native drawing'
 row_pixels=sum(n for n,color in image.crop((18,190,880,390)).getcolors(862*200) if color!=(245,245,247))
 if name.startswith('up-page-'):assert row_pixels>100,'no actual peer rows drawn'
 else:assert row_pixels==0,'stale peer pixels remained in '+name
 return path
def launch():
 global client
 with open(out/'client.log','w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'devices'],env=env,stdout=log,stderr=log)
def window():
 result=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet devices: read-only'],env=env,capture_output=True,text=True)
 return result.stdout.strip().splitlines()[-1] if result.returncode==0 and result.stdout.strip() else None
try:
 with open(out/'display','w') as fd,open(out/'x.log','w') as log:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
 number=poll(lambda:(out/'display').read_text().strip(),'X display');env['DISPLAY']=':'+number;env.pop('WAYLAND_DISPLAY',None)
 invalid=subprocess.run([str(out/'mnet'),'--state',str(state),'devices','unexpected'],env=env,capture_output=True,timeout=5)
 assert invalid.returncode!=0 and not server.requests,'extra args read API'
 launch();wid=poll(window,'native window');phase('up');capture('up-page-one')
 assert len(server.requests)==1
 # Mouse activation must belong to one exact enabled target and snapshot.
 action('windowfocus',wid,'mousemove','--window',wid,'520','420','mousedown','1','mousemove','--window',wid,'520','465','mouseup','1')
 time.sleep(.05);assert title().endswith('page1') and len(server.requests)==1,'outside press became Next on inside release'
 action('mousemove','--window',wid,'520','465','mousedown','1','mousemove','--window',wid,'120','465','mouseup','1')
 time.sleep(.05);assert title().endswith('page1') and len(server.requests)==1,'Next press rebound to Refresh'
 action('mousemove','--window',wid,'520','465','mousedown','1','mousemove','--window',wid,'520','420','mouseup','1')
 time.sleep(.05);assert title().endswith('page1') and len(server.requests)==1,'inside press activated outside release'
 action('windowfocus',wid,'mousemove','--window',wid,'520','465','click','1');poll(lambda:title().endswith('page2'),'page two');capture('up-page-two')
 action('click','1');poll(lambda:title().endswith('page3'),'page three');capture('up-page-three')
 assert len(server.requests)==1,'pagination fetched daemon or polled'
 # Snapshot-only page keys visit endpoints and clamp without an API read.
 action('key','Home');poll(lambda:title().endswith('page1'),'Home first page')
 action('key','Page_Down');poll(lambda:title().endswith('page2'),'PageDown next page')
 action('key','End');poll(lambda:title().endswith('page3'),'End last page');capture('up-page-key-end')
 action('key','Page_Down');assert title().endswith('page3'),'PageDown exceeded last page'
 action('key','Page_Up');poll(lambda:title().endswith('page2'),'PageUp previous page')
 action('key','End');poll(lambda:title().endswith('page3'),'End restores last page')
 assert len(server.requests)==1,'page key fetched daemon'
 # Actual keyboard-only focus/activation. Tab skips unavailable controls;
 # Enter/Space paginate locally, never select a peer or fetch status.
 action('key','shift+Tab');time.sleep(.05)
 subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(out/'keyboard-close-focus.png')],check=True)
 action('key','Tab','Tab','Return');poll(lambda:title().endswith('page2'),'keyboard previous Enter')
 action('key','space');poll(lambda:title().endswith('page1'),'keyboard previous Space')
 action('key','Tab','Tab','space');poll(lambda:title().endswith('page2'),'keyboard next Space')
 assert len(server.requests)==1,'keyboard pagination fetched daemon'
 # A pending activation must not migrate from Next to Close through Tab.
 action('keydown','Return','key','Tab','keyup','Return');time.sleep(.05)
 assert client.poll() is None and title().endswith('page2'),'Tab rebound a pending key to Close'
 focus_fixture=subprocess.Popen(['xmessage','-title','Mnet local focus fixture','-buttons','OK','-geometry','200x80+0+0','Local fixture only'],env=env,stdout=(out/'focus-fixture.log').open('w'),stderr=subprocess.STDOUT)
 def focus_window():
  result=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet local focus fixture$'],env=env,capture_output=True,text=True)
  return result.stdout.strip().splitlines()[-1] if result.returncode==0 and result.stdout.strip() else None
 other=poll(focus_window,'independent focus window')
 action('windowfocus',wid,'mousemove','--window',wid,'760','465','mousedown','1','windowfocus',other,'mouseup','1','windowfocus',wid)
 time.sleep(.05);assert client.poll() is None,'focus loss retained pending mouse Close'
 action('windowfocus',wid,'keydown','space','windowfocus',other,'keyup','space','windowfocus',wid)
 time.sleep(.05);assert client.poll() is None and title().endswith('page2'),'focus loss kept pending Close'
 focus_fixture.terminate();focus_fixture.wait(timeout=2);focus_fixture=None

 # Unpressed releases do not activate Close, even with focus on its button.
 action('keyup','space');time.sleep(.05);assert client.poll() is None,'unpressed Space closed window'

 server.payload=dict(up,status='down');action('key','Tab','Return');phase('down',2);capture('down-no-old-peers')
 server.payload=dict(up,status='waiting-approval');refresh('waiting-approval',3);capture('approval-no-link')
 server.code=500;refresh('unavailable',4);capture('unavailable-no-old-peers')
 server.code=200;server.payload=b'{"status":"up"}{}';refresh('unavailable',5);capture('malformed-no-old-peers')
 server.payload=up;refresh('up',6);count=len(server.requests)
 action('mousemove','--window',wid,'120','465','mousedown','1')
 time.sleep(31);phase('expired');capture('expired-no-old-peers');assert len(server.requests)==count,'automatic API polling'
 action('mouseup','1');time.sleep(.05);assert len(server.requests)==count,'expired snapshot retained mouse Refresh'
 server.delay=2;server.entered.clear();started=time.monotonic();action('key','r');phase('loading');assert server.entered.wait(2);capture('loading-no-old-peers')
 action('key','r','r','r');time.sleep(.05);assert len(server.requests)==count+1,'concurrent refreshes'
 phase('up');server.delay=0
 # The real response took two seconds. Rows must expire from request start,
 # not gain another thirty seconds when that delayed body arrives.
 time.sleep(max(0,31.2-(time.monotonic()-started)))
 assert '[expired]' in title(),'delayed response extended snapshot age'
 capture('delayed-read-expired');assert len(server.requests)==count+1,'delayed evidence expiry polled'
 before_refresh=len(server.requests)
 action('mousemove','--window',wid,'120','465','mousedown','1','key','r');phase('up',8)
 action('mouseup','1');time.sleep(.05);assert len(server.requests)==before_refresh+1,'new snapshot rebound held mouse Refresh'
 server.delay=1
 phase('up');server.entered.clear();action('key','r');phase('loading');assert server.entered.wait(2)
 before=time.monotonic();action('key','Escape');assert client.wait(timeout=2)==0;client=None;assert time.monotonic()-before<2,'close failed to cancel status'
 assert not server.bad and all(path=='/localapi/status' and credential==token for path,credential in server.requests)
 # Real HTTP service failure and capability refusal have distinct fixed copy.
 # Neither response body nor old rows can appear; a manual recovery stays local.
 server.delay=0;server.code=503;launch();wid=poll(window,'service failure window');phase('unavailable',1);capture('service-unavailable')
 server.code=403;action('windowfocus',wid);refresh('unavailable',2);capture('capability-refused')
 server.code=200;server.payload=up;refresh('up',3);capture('up-page-error-recovery')
 action('key','Escape');assert client.wait(timeout=2)==0;client=None
 # Removing the capability cannot display a cached successful snapshot.
 server.delay=0;token_file.unlink();count=len(server.requests);launch();wid=poll(window,'missing capability window');phase('unavailable');capture('missing-capability');assert len(server.requests)==count
 action('windowsize',wid,'880','480');assert client.wait(timeout=2)!=0;client=None
 token_file.write_text(token);token_file.chmod(0o600);server.payload=up
 launch();wid=poll(window,'close-button window');phase('up');action('windowfocus',wid,'mousemove','--window',wid,'760','465','click','1');assert client.wait(timeout=2)==0;client=None
 launch();wid=poll(window,'keyboard close window');phase('up');action('windowfocus',wid,'key','shift+Tab','space');assert client.wait(timeout=2)==0;client=None
 server.delay=1;server.entered.clear();launch();wid=poll(window,'termination window');phase('loading');assert server.entered.wait(2);client.terminate();assert client.wait(timeout=2)==0;client=None
 for mode in ('ready','pending','transient'):
  server.delay=1 if mode=='pending' else 0;server.entered.clear();launch();wid=poll(window,'root '+mode+' window');phase('loading' if mode=='pending' else 'up')
  assert server.entered.wait(2),'no request before root resize'
  if mode!='pending':capture('up-page-before-root-'+mode)
  try:
   subprocess.run(['xrandr','--output','screen','--off','--fb','800x600'],env=env,check=True)
   if mode=='transient':subprocess.run(['xrandr','--fb','1024x768','--output','screen','--mode','1024x768'],env=env,check=True)
   assert client.wait(timeout=2)!=0,'root resize did not retire devices';client=None
   assert 'root geometry changed' in (out/'client.log').read_text()
  finally:subprocess.run(['xrandr','--fb','1024x768','--output','screen','--mode','1024x768'],env=env,check=True)
  assert not window(),'root restoration retained device window'
  after=len(server.requests);time.sleep(.05);assert len(server.requests)==after,'root restoration read daemon'
  print('PASS actual device root '+mode+' resize joins current/pending owner and cannot resume',flush=True)
 count=len(server.requests)
 unavailable=subprocess.run([str(out/'mnet'),'--state',str(state),'devices'],env=env|{'DISPLAY':'not-a-display'},capture_output=True,timeout=5)
 assert unavailable.returncode!=0 and len(server.requests)==count,'missing X11 display contacted daemon or silently fell back'
 print('PASS actual built native Mnet device window: authenticated GET-only snapshots, Tab/Shift+Tab enabled focus and exact Enter/Space release, focus-loss/Tab cancel pending action, exact matched mouse targets with focus/expiry/snapshot cancellation, keyboard/mouse local pagination including Home/End/PageUp/PageDown,19 peers/three pages, no pagination fetch/polling, Down/approval/error/malformed/loading/expiry erase old peers, repeated Refresh coalesced, Escape/Close/SIGTERM cancel or end cleanly, missing capability/display sends no request, unsupported resize ends; private synthetic fixture only',flush=True)
finally:
 if focus_fixture is not None and focus_fixture.poll() is None:focus_fixture.terminate();focus_fixture.wait(timeout=2)
 if client is not None and client.poll() is None:client.terminate();client.wait(timeout=4)
 server.shutdown();server.server_close();thread.join(timeout=3)
 token_file.unlink(missing_ok=True)
 if x is not None:x.terminate();x.wait(timeout=3)
