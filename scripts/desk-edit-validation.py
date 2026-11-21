#!/usr/bin/env python3
"""Actual built offline Shared Desk arrangement editor on disposable X11.

Synthetic same-host geometry only: no pairing, run, daemon or identity path is
exercised. The editor can only edit an in-memory copy and create one new file.
"""
import json,os,pathlib,socketserver,subprocess,tempfile,threading,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-edit.'));print(out,flush=True)
subprocess.run(['go','build','-race','-o',str(out/'mnet'),'./cmd/mnet'],check=True)
state=out/'state';state.mkdir(mode=0o700)
class Trap(socketserver.UnixStreamServer):pass
class Handler(socketserver.BaseRequestHandler):
 def handle(self):self.server.calls+=1
server=Trap(str(state/'mnetd.sock'),Handler);server.calls=0
thread=threading.Thread(target=server.serve_forever);thread.start()
source=dict(id='source-display',device='source-device',x=-100,y=0,width=100,height=100,pixel_width=1000,pixel_height=1000)
target=dict(id='recipient-display',device='recipient-device',x=0,y=25,width=100,height=50,pixel_width=1500,pixel_height=1500)
layout=out/'layout.json';layout.write_text(json.dumps(dict(version=1,displays=[source,target])))
original=layout.read_bytes();x=None;client=None;env=os.environ.copy();wid=None
size=os.environ.get('MNET_DESK_EDIT_DISPLAY_SIZE','1024x768x24')
if size not in ('1024x768x24','2048x1536x24'):raise SystemExit('unsupported disposable size')
def poll(fn,label,seconds=5):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  value=fn()
  if value:return value
  if client is not None and client.poll() is not None:raise AssertionError(label+': '+(out/'client.log').read_text())
  time.sleep(.02)
 raise AssertionError('deadline '+label)
def action(*args):subprocess.run(['xdotool',*args],env=env,check=True)
def window(pattern='^Mnet Shared Desk: offline layout editor'):
 r=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env,capture_output=True,text=True)
 return r.stdout.strip().splitlines()[-1] if r.returncode==0 and r.stdout.strip() else None
def title():return subprocess.check_output(['xdotool','getwindowname',wid],env=env,text=True).strip()
def capture(name):
 path=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(path)],check=True)
 image=Image.open(path).convert('RGB');assert image.size==(900,730);return image
edited=out/'edited.json'
def launch(outpath=edited):
 global client,wid
 with (out/'client.log').open('w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'desk','layout-edit',str(layout),'--out',str(outpath)],env=env,stdout=log,stderr=log)
 wid=poll(window,'offline editor');action('windowfocus',wid)
try:
 with (out/'display').open('w') as fd,(out/'x.log').open('w') as log:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0',size],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
 env['DISPLAY']=':'+poll(lambda:(out/'display').read_text().strip(),'display');env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 # A static offline editor cannot block opening a writerless FIFO or read an
 # endless device. Reject nonregular input before any window or daemon I/O.
 fifo=out/'not-a-layout.fifo';os.mkfifo(fifo)
 for path in (fifo,out,'/dev/zero'):
  refused=subprocess.run([str(out/'mnet'),'--state',str(state),'desk','layout-edit',str(path),'--out',str(edited)],env=env,capture_output=True,text=True,timeout=2)
  assert refused.returncode!=0 and 'regular' in refused.stderr,'nonregular offline file not refused'
 assert not window() and server.calls==0,'invalid file created a window or daemon request'
 for args in (['layout-edit'],['layout-edit',str(layout)],['layout-edit',str(layout),'--out'],['layout-edit',str(layout),str(edited)],['layout-edit',str(layout),'--out',str(edited),'extra']):
  r=subprocess.run([str(out/'mnet'),'--state',str(state),'desk',*args],env=env,capture_output=True,text=True,timeout=3);assert r.returncode!=0 and 'usage:' in r.stderr,args
 launch();poll(lambda:'sel0/2 edgeright x-100 y0 w100 h100 pw1000 ph1000 undo0 dirtyfalse validtrue confirmfalse' in title(),'initial exact state');capture('initial')
 # Arrows move the selected display one layout unit, Shift ten. The immutable
 # opened snapshot ignores a later file replacement.
 layout.write_text(json.dumps(dict(version=1,displays=[dict(source,x=5000),target])))
 # Moves run along the shared edge so the arrangement stays linked and valid.
 action('key','Down');poll(lambda:'x-100 y1 w100' in title(),'one-unit move')
 for _ in range(2):action('key','shift+Down')
 poll(lambda:'x-100 y21 w100 h100 pw1000 ph1000 undo3 dirtytrue validtrue' in title(),'shift moves and undo depth');capture('moved')
 # Mouse activation needs a matching press/release on one enabled target.
 action('mousemove','--window',wid,'60','505','mousedown','1','mousemove','--window',wid,'830','615','mouseup','1');time.sleep(.05)
 assert 'x-100 y21 w100' in title(),'cross-target release activated a control'
 action('mousemove','--window',wid,'830','505','click','1');poll(lambda:'x-100 y11 w100 h100 pw1000 ph1000 undo2' in title(),'undo via mouse')
 action('mousemove','--window',wid,'830','505','click','1');action('mousemove','--window',wid,'830','505','click','1')
 poll(lambda:'x-100 y0 w100 h100 pw1000 ph1000 undo0 dirtyfalse' in title(),'undo back to clean')
 action('key','Escape');assert client.wait(timeout=3)==0;client=None
 layout.write_bytes(original)
 # Attach a new display to the left edge, then edit its native pixels.
 launch();poll(lambda:'dirtyfalse validtrue' in title(),'fresh editor')
 action('mousemove','--window',wid,'250','505','click','1');poll(lambda:'edgeleft' in title(),'left placement edge')
 action('mousemove','--window',wid,'640','505','click','1');poll(lambda:'sel2/3' in title() and 'x-200 y0 w100 h100 pw1000 ph1000' in title() and 'validtrue' in title(),'attached display selected');capture('added')
 action('mousemove','--window',wid,'160','615','click','1');poll(lambda:'pw1001' in title(),'native pixel width edit')
 action('key','Down');poll(lambda:'x-200 y1 w100' in title(),'new display move along shared edge')
 # Save creates exactly one new file; the source is never modified.
 action('mousemove','--window',wid,'730','615','click','1');assert client.wait(timeout=3)==0;client=None
 saved=json.loads(edited.read_bytes())
 assert len(saved['displays'])==3 and saved['displays'][2]['x']==-200 and saved['displays'][2]['y']==1 and saved['displays'][2]['pixel_width']==1001,'saved geometry mismatch'
 assert saved['displays'][0]==source and saved['displays'][1]==target,'unedited displays changed'
 assert layout.read_bytes()==original and server.calls==0,'save touched the source or contacted a daemon'
 import stat as statmod;assert statmod.S_IMODE(edited.stat().st_mode)==0o600,'saved file not owner-only'
 # A second save to the same path refuses instead of replacing. The move runs
 # along the shared edge so the arrangement stays valid and Save stays enabled.
 launch();action('key','Down');poll(lambda:'x-100 y1 w100 h100 pw1000 ph1000 undo1 dirtytrue validtrue' in title(),'second dirty')
 action('mousemove','--window',wid,'730','615','click','1');poll(lambda:'already exists' in title(),'existing destination refused');capture('save-exists')
 assert client.poll() is None,'failed save closed the editor'
 # An overlapping arrangement is invalid; Save is disabled and explains why.
 action('key','Right')
 poll(lambda:'x-99 y1 w100 h100 pw1000 ph1000 undo2 dirtytrue validfalse' in title(),'overlap invalid');capture('overlap-invalid')
 action('mousemove','--window',wid,'730','615','click','1');poll(lambda:'notice[Save needs unsaved changes and a valid arrangement]' in title(),'disabled save notice')
 assert client.poll() is None,'invalid arrangement saved'
 for _ in range(2):action('mousemove','--window',wid,'830','505','click','1')
 poll(lambda:'dirtyfalse validtrue' in title(),'undo back to original')
 action('key','Escape');assert client.wait(timeout=3)==0;client=None
 # Closing with unsaved changes asks explicitly; discarding writes nothing.
 discarded=out/'discarded.json'
 launch(discarded);action('key','Right');poll(lambda:'dirtytrue' in title(),'dirty before close')
 action('key','Escape');poll(lambda:'confirmtrue' in title(),'discard confirmation');capture('confirm')
 action('key','Escape');poll(lambda:'confirmfalse' in title(),'confirmation canceled')
 action('key','Escape');poll(lambda:'confirmtrue' in title(),'confirmation again')
 action('mousemove','--window',wid,'260','322','click','1');poll(lambda:'confirmfalse' in title(),'keep editing keeps changes')
 assert 'dirtytrue' in title() and client.poll() is None
 action('key','Escape');poll(lambda:'confirmtrue' in title(),'third confirmation')
 action('mousemove','--window',wid,'100','322','click','1');assert client.wait(timeout=3)==0;client=None
 assert not discarded.exists(),'discard wrote the destination'
 assert layout.read_bytes()==original and server.calls==0
 # Keyboard-only journey: Tab focuses controls; matched Enter activates.
 launch(out/'keyboard.json');action('key','Tab');poll(lambda:capture('keyboard-focus').getpixel((23,489))==(37,99,199),'actual keyboard focus ring')
 action('keydown','Return','key','Tab','keyup','Return');time.sleep(.05);assert 'sel0/2' in title(),'Tab reassigned a pending key'
 action('key','shift+Tab');action('key','shift+Tab');poll(lambda:'focus22' in title(),'reverse tab to Close')
 action('key','Return');assert client.wait(timeout=3)==0,'clean close via keyboard';client=None
 assert not (out/'keyboard.json').exists(),'close saved without changes'
 # SIGTERM takes the normal owner-cancellation path and writes nothing.
 launch(out/'terminated.json');client.terminate();assert client.wait(timeout=2)==0,'SIGTERM bypassed owned cancellation';client=None
 assert not (out/'terminated.json').exists() and layout.read_bytes()==original and server.calls==0
 # Root geometry change retires the fixed editor window; restoring cannot revive.
 launch(out/'resized.json')
 try:
  subprocess.run(['xrandr','--output','screen','--off','--fb','800x600'],env=env,check=True)
  assert client.wait(timeout=2)!=0,'root resize accepted';client=None
  assert 'root geometry changed' in (out/'client.log').read_text(),'missing root-resize diagnostic'
 finally:
  original_size=size.rsplit('x',1)[0]
  subprocess.run(['xrandr','--fb',original_size,'--output','screen','--mode',original_size],env=env,check=True)
 assert not window() and not (out/'resized.json').exists(),'root restoration retained editor or wrote'
 # Fail closed on child resize, without a terminal fallback.
 launch(out/'childsize.json');action('windowsize',wid,'901','730');assert client.wait(timeout=3)!=0;client=None
 assert 'unsupported resize' in (out/'client.log').read_text() and not (out/'childsize.json').exists()
 print('PASS actual offline arrangement editor: exact unit/Shift moves, undo, edge attach, native pixel edits, create-only synced save (0600, source untouched), existing-destination refusal, invalid-overlap disabled save, explicit discard confirmation, gesture/focus/Tab cancellation, immutable snapshot, SIGTERM/root-resize/child-resize teardown, zero daemon calls. Synthetic same-host geometry only.',flush=True)
finally:
 if client is not None and client.poll() is None:client.terminate();client.wait(timeout=3)
 if x is not None:x.terminate();x.wait(timeout=3)
 server.shutdown()
