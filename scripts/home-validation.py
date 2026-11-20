#!/usr/bin/env python3
"""Actual built engineering home routes to real child windows, never a mock UI."""
import http.server,json,os,pathlib,secrets,socketserver,subprocess,tempfile,threading,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-home.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
state=out/'state';state.mkdir(mode=0o700);token=secrets.token_hex(32);(state/'local-api-token').write_text(token);(state/'local-api-token').chmod(0o600)
class Server(socketserver.ThreadingMixIn,socketserver.UnixStreamServer):daemon_threads=True
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_POST(self):self.server.bad.append('mutation');self.send_error(403)
 def do_GET(self):
  self.server.reads.append(self.path)
  if self.headers.get('X-Mnet-Local-Token')!=token:self.server.bad.append('auth');self.send_error(403);return
  if self.path=='/localapi/status':payload={'status':'down','node_name':'Synthetic home fixture'}
  elif self.path=='/localapi/session-peers':payload=[]
  else:self.server.bad.append(self.path);self.send_error(403);return
  data=json.dumps(payload).encode();self.send_response(200);self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
server=Server(str(state/'mnetd.sock'),Handler);server.reads=[];server.bad=[];thread=threading.Thread(target=server.serve_forever);thread.start()
env=os.environ.copy();env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
layout=out/'layout.json';layout.write_text(json.dumps({'version':1,'displays':[{'id':'local','device':'synthetic-a','x':0,'y':0,'width':100,'height':100,'pixel_width':1000,'pixel_height':1000},{'id':'neighbor','device':'synthetic-b','x':100,'y':0,'width':100,'height':100,'pixel_width':1000,'pixel_height':1000}]}))
x=None;client=None

def poll(fn,label):
 end=time.monotonic()+6
 while time.monotonic()<end:
  value=fn()
  if value:return value
  if client is not None and client.poll() is not None:raise AssertionError('early exit '+label+': '+(out/'client.log').read_text())
  time.sleep(.02)
 raise AssertionError('deadline '+label)
def window(pattern):
 result=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env,text=True,capture_output=True)
 return result.stdout.strip().splitlines()[-1] if result.returncode==0 and result.stdout.strip() else None
def action(*args):subprocess.run(['xdotool',*args],env=env,check=True)
def screenshot(wid,name):
 path=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(path)],check=True);image=Image.open(path).convert('RGB');assert image.size==(900,560);assert image.getpixel((20,110))==(28,42,59);assert image.getpixel((680,486))==(48,71,97);return path
def launch(with_layout=False):
 global client
 with (out/'client.log').open('w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'open',*(['--layout',str(layout)] if with_layout else [])],env=env,stdout=log,stderr=log)
try:
 with (out/'display').open('w') as fd,(out/'x.log').open('w') as log:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
 env['DISPLAY']=':'+poll(lambda:(out/'display').read_text().strip(),'display')
 launch();home=poll(lambda:window('^Mnet: three tools'),'home');screenshot(home,'home');assert not server.reads
 action('windowfocus',home,'mousemove','--window',home,'100','370','click','1');time.sleep(.1);assert not server.reads and window('^Mnet: three tools'),'disabled layout activated'
 # Every transition closes and joins the prior home. A child Escape returns a
 # fresh home; returning never repeats local API reads or reuses a selection.
 action('mousemove','--window',home,'100','140','click','1');device=poll(lambda:window('^Mnet devices: read-only \\[down\\]'),'device child');assert server.reads==['/localapi/status']
 assert not window('^Mnet: three tools'),'home remained active behind child'
 action('windowfocus',device,'key','Escape');home=poll(lambda:window('^Mnet: three tools'),'returned home');screenshot(home,'returned-home');before=list(server.reads);time.sleep(.1);assert server.reads==before
 action('windowfocus',home,'key','Tab','Tab','Return');chooser=poll(lambda:window('^Mnet Remote Desktop: choose'),'desktop child');assert not window('^Mnet: three tools')
 action('windowfocus',chooser,'key','Escape');home=poll(lambda:window('^Mnet: three tools'),'desktop return');assert not server.bad
 # Pending keyboard action cannot migrate through Tab to another route.
 before=list(server.reads);action('windowfocus',home,'key','Tab','keydown','Return','key','Tab','keyup','Return');time.sleep(.1);assert window('^Mnet: three tools') and server.reads==before
 action('key','Escape');assert client.wait(timeout=3)==0
 launch(True);home=poll(lambda:window('^Mnet: three tools'),'layout home');screenshot(home,'layout-enabled-home');before=list(server.reads)
 action('windowfocus',home,'mousemove','--window',home,'100','370','click','1');preview=poll(lambda:window('^Mnet Shared Desk'),'actual layout child')
 assert server.reads==before,'offline layout consulted daemon'
 action('windowfocus',preview,'key','Escape');home=poll(lambda:window('^Mnet: three tools'),'layout return');assert server.reads==before
 client.terminate();assert client.wait(timeout=3)==0;assert not server.bad
 print('PASS actual home routes devices/chooser/offline layout, child return without retry, zero home API reads, disabled layout, pending-key cancellation and SIGTERM join',flush=True)
finally:
 if client is not None and client.poll() is None:client.kill();client.wait()
 if x is not None:x.terminate();x.wait(timeout=3)
 server.shutdown();server.server_close();thread.join()
 print('Artifacts retained: '+str(out),flush=True)
