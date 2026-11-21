#!/usr/bin/env python3
"""Actual home-driven Shared Desk arrangements journey on disposable X11.

Synthetic same-host geometry only: no pairing, run, daemon or identity path is
exercised. The window only manages offline draft files; saving always creates
one new file in the private store.
"""
import json,os,pathlib,socketserver,stat,subprocess,tempfile,threading,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-home.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
state=out/'state';state.mkdir(mode=0o700)
class Trap(socketserver.UnixStreamServer):pass
class Handler(socketserver.BaseRequestHandler):
 def handle(self):self.server.calls+=1
server=Trap(str(state/'mnetd.sock'),Handler);server.calls=0
thread=threading.Thread(target=server.serve_forever);thread.start()
x=None;client=None;env=os.environ.copy();wid=None
def poll(fn,label,seconds=5):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  value=fn()
  if value:return value
  if client is not None and client.poll() is not None:raise AssertionError(label+': '+(out/'client.log').read_text())
  time.sleep(.02)
 raise AssertionError('deadline '+label)
def action(*args):subprocess.run(['xdotool',*args],env=env,check=True)
def window(pattern):
 r=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env,capture_output=True,text=True)
 return r.stdout.strip().splitlines()[-1] if r.returncode==0 and r.stdout.strip() else None
def title():return subprocess.check_output(['xdotool','getwindowname',wid],env=env,text=True).strip()
def capture(name,w):
 path=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',w,str(path)],check=True)
 return Image.open(path).convert('RGB')
store=state/'desk-layouts'
try:
 with (out/'display').open('w') as fd,(out/'x.log').open('w') as log:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
 env['DISPLAY']=':'+poll(lambda:(out/'display').read_text().strip(),'display');env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 with (out/'client.log').open('w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'open'],env=env,stdout=log,stderr=log)
 home=poll(lambda:window('^Mnet: three tools'),'home')
 action('windowfocus',home,'mousemove','--window',home,'100','370','click','1')
 wid=poll(lambda:window('^Mnet Shared Desk: arrangements'),'arrangements window');action('windowfocus',wid)
 poll(lambda:'rows0 sel-1 pinnedfalse' in title(),'empty arrangements state')
 image=capture('arrangements-empty',wid);assert image.size==(900,560)
 assert image.getpixel((30,120))==(255,255,255),'empty list panel not painted'
 assert not store.exists(),'browsing created the store directory'
 assert server.calls==0,'arrangements consulted a daemon'
 # New starter opens the seeded editor; saving creates one private store file.
 action('mousemove','--window',wid,'90','474','click','1')
 wid=poll(lambda:window('^Mnet Shared Desk: offline layout editor'),'starter editor');action('windowfocus',wid)
 poll(lambda:'dirtytrue validtrue' in title() and 'startertrue' in title(),'starter seeded from home')
 action('key','Down');poll(lambda:'y1' in title(),'starter edit')
 action('mousemove','--window',wid,'730','615','click','1')
 wid=poll(lambda:window('^Mnet Shared Desk: arrangements'),'arrangements after save');action('windowfocus',wid)
 poll(lambda:'rows1 sel0' in title(),'saved draft listed');capture('arrangements-saved',wid)
 saved=store/'arrangement-1.json'
 assert saved.exists() and stat.S_IMODE(saved.stat().st_mode)==0o600,'saved draft missing or not owner-only'
 assert stat.S_IMODE(store.stat().st_mode)==0o700,'store directory not private'
 data=json.loads(saved.read_bytes());assert [d['id'] for d in data['displays']]==['this-display','next-display'] and data['displays'][0]['y']==1,'saved starter geometry wrong'
 assert server.calls==0,'save consulted a daemon'
 # Inspect opens the read-only inspector on the selected draft.
 action('mousemove','--window',wid,'240','474','click','1')
 preview=poll(lambda:window('^Mnet Shared Desk: offline layout'),'inspect child')
 ptitle=lambda:subprocess.check_output(['xdotool','getwindowname',preview],env=env,text=True).strip()
 poll(lambda:'placeholdertrue' in ptitle(),'inspector marks starter placeholder labels')
 subprocess.run(['import','-display',env['DISPLAY'],'-window',preview,str(out/'inspect-placeholder.png')],check=True)
 action('windowfocus',preview,'key','Escape')
 wid=poll(lambda:window('^Mnet Shared Desk: arrangements'),'arrangements after inspect');action('windowfocus',wid)
 # Edit opens the file-backed editor; a clean close saves nothing new.
 action('mousemove','--window',wid,'390','474','click','1')
 wid=poll(lambda:window('^Mnet Shared Desk: offline layout editor'),'edit child');action('windowfocus',wid)
 poll(lambda:'dirtyfalse' in title() and 'starterfalse' in title(),'file-backed edit from home')
 action('key','Escape')
 wid=poll(lambda:window('^Mnet Shared Desk: arrangements'),'arrangements after clean edit');action('windowfocus',wid)
 assert len(list(store.glob('*.json')))==1,'clean close wrote a file'
 # Escape returns home; closing the journey makes no daemon call.
 action('windowfocus',wid,'key','Escape');poll(lambda:window('^Mnet: three tools'),'home after journey')
 assert server.calls==0
 client.terminate();assert client.wait(timeout=3)==0;client=None
 # An invalid draft is listed honestly and refused with an in-window notice.
 store.mkdir(exist_ok=True);(store/'broken.json').write_text('{not json')
 with (out/'client.log').open('w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'open'],env=env,stdout=log,stderr=log)
 home=poll(lambda:window('^Mnet: three tools'),'home with invalid draft')
 action('windowfocus',home,'mousemove','--window',home,'100','370','click','1')
 wid=poll(lambda:window('^Mnet Shared Desk: arrangements'),'arrangements with invalid draft');action('windowfocus',wid)
 poll(lambda:'rows2' in title(),'invalid draft listed')
 action('key','Down')  # select broken.json (sorted after arrangement-1.json)
 poll(lambda:'sel1' in title(),'invalid draft selected');capture('arrangements-invalid',wid)
 action('mousemove','--window',wid,'240','474','click','1');time.sleep(.15)
 poll(lambda:'could not be opened' in title(),'invalid draft refused in-window')
 assert window('^Mnet Shared Desk: arrangements') and server.calls==0,'invalid draft closed the window or consulted a daemon'
 # Delete requires a second explicit press, selection change cancels it, and only the named draft goes.
 action('mousemove','--window',wid,'540','474','click','1')
 poll(lambda:'armedtrue' in title(),'delete armed');capture('arrangements-delete-armed',wid)
 action('key','Up');poll(lambda:'armedfalse' in title() and 'sel0' in title(),'selection change cancels pending delete')
 action('key','Down');poll(lambda:'sel1' in title(),'invalid draft reselected')
 action('mousemove','--window',wid,'540','474','click','1');poll(lambda:'armedtrue' in title(),'delete re-armed')
 action('mousemove','--window',wid,'540','474','click','1')
 poll(lambda:'rows1' in title() and 'Deleted broken.json' in title(),'invalid draft deleted after confirm')
 assert not (store/'broken.json').exists() and (store/'arrangement-1.json').exists(),'delete removed the wrong files'
 capture('arrangements-deleted',wid)
 assert server.calls==0,'delete consulted a daemon'
 action('key','Escape');poll(lambda:window('^Mnet: three tools'),'home after invalid refusal')
 client.terminate();assert client.wait(timeout=3)==0;client=None
 print('PASS actual home arrangements journey: always-enabled card, empty-store browsing creates nothing, starter creation saving one private 0600 draft, inspect/edit child windows, clean close writes nothing, invalid draft listed and refused in-window, zero daemon calls. Synthetic same-host geometry only.',flush=True)
finally:
 if client is not None and client.poll() is None:client.terminate();client.wait(timeout=3)
 if x is not None:x.terminate();x.wait(timeout=3)
 server.shutdown()
