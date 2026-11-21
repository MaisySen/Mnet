#!/usr/bin/env python3
"""Actual built engineering home routes to real child windows, never a mock UI."""
import ctypes,ctypes.util,http.server,json,os,pathlib,secrets,socketserver,subprocess,tempfile,threading,time
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
 path=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(path)],check=True);image=Image.open(path).convert('RGB');assert image.size==(900,560);assert image.getpixel((20,110))==(255,255,255);assert image.getpixel((680,486))==(255,255,255);assert image.getpixel((24,18))==(16,62,137);return path
def launch(with_layout=False,origin=None):
 global client
 with (out/'client.log').open('w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'open',*(['--layout',str(layout)] if with_layout else []),*(['--multies-server',origin] if origin else [])],env=env,stdout=log,stderr=log)
try:
 with (out/'display').open('w') as fd,(out/'x.log').open('w') as log:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
 env['DISPLAY']=':'+poll(lambda:(out/'display').read_text().strip(),'display')
 launch();home=poll(lambda:window('^Mnet: three tools'),'home');screenshot(home,'home');assert not server.reads
 action('windowfocus',home,'mousemove','--window',home,'200','510','click','1');time.sleep(.1);assert window('^Mnet: three tools') and not server.reads,'unconfigured review activated'
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
 # Explicit configured review is an opt-in child. Opening/closing without
 # Start must not prepare identity, open the browser or read the local API.
 launch(origin='https://multies.example.test');home=poll(lambda:window('^Mnet: three tools'),'configured home');screenshot(home,'configured-home');before=list(server.reads)
 # A native NoSymbol keyboard release must not complete the pending mouse
 # press on Review. Keep the actual left mouse press held through the release.
 subprocess.run(['xmodmap','-e','keycode 255 = NoSymbol'],env=env,check=True)
 action('windowfocus',home,'mousemove','--window',home,'200','510','mousedown','1')
 old_display=os.environ.get('DISPLAY');os.environ['DISPLAY']=env['DISPLAY']
 xlib=ctypes.CDLL(ctypes.util.find_library('X11'));xtest=ctypes.CDLL(ctypes.util.find_library('Xtst'))
 xlib.XOpenDisplay.argtypes=[ctypes.c_char_p];xlib.XOpenDisplay.restype=ctypes.c_void_p
 xlib.XSync.argtypes=[ctypes.c_void_p,ctypes.c_int];xlib.XCloseDisplay.argtypes=[ctypes.c_void_p]
 xtest.XTestFakeKeyEvent.argtypes=[ctypes.c_void_p,ctypes.c_uint,ctypes.c_int,ctypes.c_ulong]
 conn=xlib.XOpenDisplay(None);assert conn
 xtest.XTestFakeKeyEvent(conn,255,0,0);xlib.XSync(conn,0);xlib.XCloseDisplay(conn)
 if old_display is None:os.environ.pop('DISPLAY',None)
 else:os.environ['DISPLAY']=old_display
 time.sleep(.1);assert window('^Mnet: three tools') and server.reads==before,'NoSymbol release completed mouse review press'
 action('mousemove','--window',home,'5','5','mouseup','1')
 # Reset focus for the separate keyboard journey below.
 action('windowfocus',home,'key','Escape');assert client.wait(timeout=3)==0
 launch(origin='https://multies.example.test');home=poll(lambda:window('^Mnet: three tools'),'fresh configured home')
 action('windowfocus',home,'key','Tab','Tab','Tab','Tab')
 # No layout means Tab visits network, desktop, close, then approval review.
 # The header explanation is focus 5, after review.
 action('keydown','Return','key','Tab','keyup','Return');time.sleep(.1)
 assert window('^Mnet: three tools') and server.reads==before,'pending approval action migrated'
 action('key','Tab','Tab','Tab','Tab','Return')
 review=poll(lambda:window('^Mnet: experimental Multies device approvals'),'configured review child')
 assert not window('^Mnet: three tools') and server.reads==before and not server.bad,'review entry made request'
 action('windowfocus',review,'key','Escape');home=poll(lambda:window('^Mnet: three tools'),'review return');screenshot(home,'review-return-home')
 assert server.reads==before and not server.bad,'review return made request'
 # The consent-map explanation is a header toggle, never a route: no daemon
 # reads, the window stays, and while open the tool routes are inert.
 action('windowfocus',home,'key','Tab','Tab','Tab','Tab','Tab','Return');time.sleep(.1)
 info=poll(lambda:window('info1'),'keyboard info panel')
 path=out/'home-info.png';subprocess.run(['import','-display',env['DISPLAY'],'-window',info,str(path)],check=True)
 image=Image.open(path).convert('RGB');assert image.size==(900,560)
 # Panel pixels: logo and white panel present, tool footer cards absent.
 assert image.getpixel((24,18))==(16,62,137) and image.getpixel((400,200))==(255,255,255),'info panel not painted'
 assert image.getpixel((20,500))==(244,246,250),'info panel kept tool footer'
 assert server.reads==before,'info panel consulted daemon'
 action('mousemove','--window',info,'100','140','click','1');time.sleep(.1)
 assert window('info1') and server.reads==before and not window('^Mnet devices'),'info panel routed a tool'
 action('key','Escape');poll(lambda:not window('info1'),'info panel close');assert window('^Mnet: three tools')
 action('windowfocus',home,'mousemove','--window',home,'620','38','click','1');poll(lambda:window('info1'),'mouse info panel')
 assert server.reads==before,'mouse info panel consulted daemon'
 action('key','Escape');poll(lambda:not window('info1'),'mouse info panel close')
 action('windowfocus',home,'mousemove','--window',home,'200','510','click','1');review=poll(lambda:window('^Mnet: experimental Multies device approvals'),'mouse review child')
 client.terminate();assert client.wait(timeout=3)==0
 assert server.reads==before and not server.bad,'review cancellation made request'
 print('PASS actual home routes, info panel toggle devices/chooser/offline layout, child return without retry, zero home API reads, disabled layout, pending-key cancellation and SIGTERM join',flush=True)
finally:
 if client is not None and client.poll() is None:client.kill();client.wait()
 if x is not None:x.terminate();x.wait(timeout=3)
 server.shutdown();server.server_close();thread.join()
 print('Artifacts retained: '+str(out),flush=True)
