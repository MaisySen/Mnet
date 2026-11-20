#!/usr/bin/env python3
"""Actual built offline Shared Desk inspector on disposable X11, no live identity."""
import json,os,pathlib,socketserver,subprocess,tempfile,threading,time
from PIL import Image
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-desk-preview.'));print(out,flush=True)
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
original=layout.read_bytes();x=None;client=None;focus_app=None;env=os.environ.copy();wid=None
size=os.environ.get('MNET_DESK_PREVIEW_DISPLAY_SIZE','1024x768x24')
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
def window(pattern='^Mnet Shared Desk: offline layout'):
 r=subprocess.run(['xdotool','search','--onlyvisible','--name',pattern],env=env,capture_output=True,text=True)
 return r.stdout.strip().splitlines()[-1] if r.returncode==0 and r.stdout.strip() else None
def title():return subprocess.check_output(['xdotool','getwindowname',wid],env=env,text=True).strip()
def capture(name):
 path=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(path)],check=True)
 image=Image.open(path).convert('RGB');assert image.size==(900,650);return image
try:
 with (out/'display').open('w') as fd,(out/'x.log').open('w') as log:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0',size],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
 env['DISPLAY']=':'+poll(lambda:(out/'display').read_text().strip(),'display');env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 def launch():
  global client,wid
  with (out/'client.log').open('w') as log:client=subprocess.Popen([str(out/'mnet'),'--state',str(state),'desk','layout-view',str(layout)],env=env,stdout=log,stderr=log)
  wid=poll(window,'offline diagram');action('windowfocus',wid)
 report=json.loads(subprocess.check_output([str(out/'mnet'),'--state',str(state),'desk','layout-check',str(layout),'--edge','source-display','right','500'],env=env,text=True));assert report['offline_only'] and not report['active'] and report['candidate']==dict(display='recipient-display',device='recipient-device',x=0,y=750)
 for args in (['layout-view'],['layout-view',str(layout),'--approve'],['run',str(layout)]):
  r=subprocess.run([str(out/'mnet'),'--state',str(state),'desk',*args],env=env,capture_output=True,text=True,timeout=3);assert r.returncode!=0 and 'usage:' in r.stderr
 launch();poll(lambda:title().endswith('edgeright pixel0 no-neighbor'),'initial exact gap');capture('gap')
 # Direct diagram selection needs one matching rectangle, not an outside or
 # different-display press. Inspecting a rectangle does not query/mutate a run.
 action('mousemove','--window',wid,'100','120','mousedown','1','mousemove','--window',wid,'550','220','mouseup','1');time.sleep(.05);assert 'display1 ' in title(),'outside press selected rectangle'
 action('mousemove','--window',wid,'300','220','mousedown','1','mousemove','--window',wid,'550','220','mouseup','1');time.sleep(.05);assert 'display1 ' in title(),'source press selected recipient on release'
 action('mousemove','--window',wid,'550','220','click','1');poll(lambda:'display2 ' in title(),'direct recipient rectangle selection');capture('mouse-selected-recipient')
 action('mousemove','--window',wid,'300','220','click','1');poll(lambda:'display1 ' in title(),'direct source selection resets native query')

 # Rounded diagram must not replace original native-pixel geometry. Exact
 # midpoint on the partially shared edge maps into the native target center.
 action('key','End');poll(lambda:title().endswith('pixel999 no-neighbor'),'other exact gap');action('key','Home')
 for _ in range(50):action('key','shift+Right')
 poll(lambda:title().endswith('pixel500 candidate=recipient-display@0,750'),'native unequal-size mapping');capture('mapped')
 # No unpressed/cross-target release, focus loss or canceled key may close it.
 action('mousemove','--window',wid,'20','560','mousedown','1','mousemove','--window',wid,'830','600','mouseup','1');assert client.poll() is None
 action('mousemove','--window',wid,'60','600','mousedown','1','mousemove','--window',wid,'830','600','mouseup','1');assert client.poll() is None
 action('key','Tab');poll(lambda:capture('keyboard-focus').getpixel((23,583))==(255,255,255),'actual keyboard focus ring')
 action('keydown','Return','key','Tab','keyup','Return');time.sleep(.05);assert 'display1 ' in title(),'Tab reassigned pending Previous key'
 focus_app=subprocess.Popen(['xmessage','-title','Offline preview focus fixture','-buttons','OK','Local focus fixture'],env=env,stdout=(out/'focus.log').open('w'),stderr=subprocess.PIPE)
 other=poll(lambda:window('^Offline preview focus fixture$'),'independent focus fixture')
 action('windowfocus',wid,'mousemove','--window',wid,'830','600','mousedown','1','windowfocus',other,'mouseup','1','windowfocus',wid);assert client.poll() is None
 action('mousemove','--window',wid,'550','220','mousedown','1','windowfocus',other,'mouseup','1','windowfocus',wid);time.sleep(.05);assert 'display1 ' in title(),'focus loss retained recipient rectangle press'
 action('mousemove','--window',wid,'550','220','mousedown','1','key','Tab','mouseup','1');time.sleep(.05);assert 'display1 ' in title(),'keyboard change retained recipient rectangle press'
 focus_app.terminate();focus_app.communicate(timeout=2);focus_app=None
 # A later file replacement cannot alter this immutable in-memory snapshot.
 layout.write_text('invalid replacement');action('key','Right');poll(lambda:title().endswith('pixel501 candidate=recipient-display@0,753'),'static opened-file snapshot');layout.write_bytes(original)
 action('key','Next');poll(lambda:'display2 ' in title(),'second display');action('mousemove','--window',wid,'260','600','click','1');poll(lambda:title().endswith('edgeleft pixel0 candidate=source-display@999,250'),'exact return edge');capture('return')
 action('key','Escape');assert client.wait(timeout=3)==0;client=None;assert layout.read_bytes()==original,'preview wrote source file'
 # Every supported display remains inspectable, including tiny projected
 # rectangles and maximum-length labels. No list item is silently omitted.
 displays=[dict(id=('display-'+str(i)).ljust(64,'x'),device=('device-'+str(i)).ljust(64,'y'),x=-1000000+i*60000,y=-1000000,width=60000,height=1,pixel_width=65535,pixel_height=1) for i in range(32)]
 layout.write_text(json.dumps(dict(version=1,displays=displays)));many=layout.read_bytes();launch()
 for _ in range(31):action('key','Next')
 poll(lambda:'display32 ' in title(),'all32 labels');action('mousemove','--window',wid,'40','225','click','1');time.sleep(.05);assert 'display32 ' in title(),'tiny diagram acquired a direct target';action('key','End');poll(lambda:'pixel0 ' in title(),'one native tangent pixel');capture('all32-last')
 action('key','Tab');poll(lambda:capture('all32-keyboard-focus').getpixel((23,583))==(255,255,255),'focus after all32')
 action('key','Escape');assert client.wait(timeout=3)==0;client=None;assert layout.read_bytes()==many
 # Fail closed on resize and actual display loss, without terminal fallback.
 layout.write_bytes(original);launch();action('windowsize',wid,'901','650');assert client.wait(timeout=3)!=0;client=None;assert 'unsupported resize' in (out/'client.log').read_text()
 launch();x.terminate();x.wait(timeout=3);x=None;assert client.wait(timeout=3)!=0;client=None;assert any(message in (out/'client.log').read_text() for message in ('display ended','mnet: EOF'))
 assert 'DATA RACE' not in (out/'client.log').read_text()
 for data in ('{}',json.dumps(dict(version=1,displays=[source,dict(target,x=-1)])),original.decode()+'{}'):
  layout.write_text(data);r=subprocess.run([str(out/'mnet'),'--state',str(state),'desk','layout-view',str(layout)],env=env,capture_output=True,text=True,timeout=3);assert r.returncode!=0
 assert server.calls==0,'offline preview contacted local daemon'
 print('PASS actual offline diagram: direct44px/unambiguous rectangle selection and gesture cancellation, unequal/offset native mapping, gaps and reverse edge; immutable read-only file;32 full labels/tiny paint; exact gestures/focus/Tab cancellation; resize/display loss end; invalid file rejected; zero daemon calls. Synthetic same-host geometry only.',flush=True)
finally:
 if focus_app is not None:focus_app.terminate();focus_app.communicate(timeout=3)
 if client is not None and client.poll() is None:client.terminate();client.wait(timeout=3)
 server.shutdown();server.server_close();thread.join(timeout=3)
 if x is not None:x.terminate();x.wait(timeout=3)
