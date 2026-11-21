#!/usr/bin/env python3
"""Actual control/two-daemon/chooser lifecycle. ChannelTUN only, NO OS TCP/session proof."""
import http.client,json,os,pathlib,socket,subprocess,tempfile,time,urllib.request,http.cookiejar
from PIL import Image
repo=pathlib.Path(__file__).resolve().parent.parent
os.chdir(repo)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-daemon-chooser.'));print(out,flush=True)
processes=[];client=None;env=os.environ.copy()
def start(args,name,extra=None):
 log=(out/(name+'.log')).open('w');p=subprocess.Popen(list(map(str,args)),stdout=log,stderr=log,env=extra);log.close();processes.append(p);return p
def poll(fn,label,seconds=15):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  try:
   result=fn()
   if result:return result
  except (OSError,ValueError,http.client.HTTPException):pass
  for p in processes:
   if p.poll() is not None:raise AssertionError('process exited during '+label+'; inspect '+str(out))
  time.sleep(.04)
 raise AssertionError('deadline '+label+'; inspect '+str(out))
class UnixHTTP(http.client.HTTPConnection):
 def __init__(self,state):super().__init__('localhost',timeout=3);self.state=state
 def connect(self):self.sock=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM);self.sock.settimeout(3);self.sock.connect(str(self.state/'mnetd.sock'))
def api(state,path='status',body=None):
 c=UnixHTTP(state)
 try:
  token=(state/'local-api-token').read_text().strip()
  c.request('GET' if body is None else 'POST','/localapi/'+path,None if body is None else json.dumps(body),{'X-Mnet-Local-Token':token,'Content-Type':'application/json'})
  r=c.getresponse();raw=r.read();assert r.status==200,'local API refusal';return json.loads(raw) if raw else None
 finally:c.close()
def window():
 r=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet Remote Desktop: choose'],env=env,capture_output=True,text=True)
 return r.stdout.strip().splitlines()[-1] if r.returncode==0 and r.stdout.strip() else None
def action(*args):subprocess.run(['xdotool',*args],env=env,check=True)
def title():return subprocess.check_output(['xdotool','getwindowname',wid],env=env,text=True).strip()
def phase(name,read):poll(lambda:'['+name+']' in title() and 'read'+str(read)+' ' in title(),name)
def capture(name):
 p=out/(name+'.png');subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(p)],check=True);im=Image.open(p).convert('RGB');assert im.size==(900,560);return im
try:
 for name,pkg in [('mnet','./cmd/mnet'),('mnetd','./cmd/mnetd'),('control','./cmd/mnet-control')]:subprocess.run(['go','build','-race','-o',str(out/name),pkg],check=True)
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 base='http://127.0.0.1:'+str(port)
 start([out/'control','-listen','127.0.0.1:'+str(port),'-base-url',base,'-data',out/'control.json','-admin-password','disposable-chooser-only'],'control')
 jar=http.cookiejar.CookieJar();opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
 def web(path,body=None):
  req=urllib.request.Request(base+path,None if body is None else json.dumps(body).encode(),{'Content-Type':'application/json'})
  with opener.open(req,timeout=3) as r:
   raw=r.read();return True if path=='/healthz' else json.loads(raw)
 poll(lambda:web('/healthz'),'control ready')
 web('/admin/api/login',{'password':'disposable-chooser-only'})
 network=web('/admin/api/networks',{'name':'disposable-chooser'})
 key=web('/admin/api/authkeys',{'network_id':network['id'],'reusable':True})['key']
 states={name:out/name for name in ('viewer-state','host-state')};daemons={}
 for name,state in states.items():
  daemons[name]=start([out/'mnetd','-state',state,'-fake-tun'],name)
  poll(lambda:api(state),'actual daemon readiness')
  api(state,'up',{'server':base,'network':'disposable-chooser','name':name,'auth_key':key})
  poll(lambda:api(state)['status']=='up','actual daemon Up')
 viewer=states['viewer-state'];host=states['host-state']
 peers=poll(lambda:api(viewer,'session-peers'),'actual control-map peer')
 host_identity=json.loads((host/'state.json').read_text())
 assert len(peers)==1 and peers[0]['Name']=='host-state' and peers[0]['PublicKey']==host_identity['public_key'],'actual peer key did not come from owned other daemon'
 identities={name:(state/'state.json').read_bytes() for name,state in states.items()}
 # Explicit inherited descriptor belongs only to this X server.
 with (out/'display').open('w') as fd,(out/'xserver.log').open('w') as log:
  p=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],pass_fds=(fd.fileno(),),stdout=log,stderr=log);processes.append(p)
 env['DISPLAY']=':'+poll(lambda:(out/'display').read_text().strip(),'X display');env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
 client=start([out/'mnet','--state',viewer,'session','browse'],'chooser',env)
 wid=poll(window,'actual daemon chooser');action('windowfocus',wid);phase('ready',1);capture('actual-daemon-ready')
 action('mousemove','--window',wid,'120','120','click','1','mousemove','--window',wid,'800','385','click','1');poll(lambda:'detailstrue' in title(),'actual key details');capture('actual-daemon-selected-details')
 api(viewer,'down',{});poll(lambda:api(viewer)['status']=='down','actual daemon Down')
 assert not api(viewer,'session-peers'),'down daemon retained peer authority'
 action('key','r');phase('unavailable',2);assert 'detailsfalse' in title(),'Down refresh kept disclosed key';capture('actual-daemon-down-cleared')
 # Neither stale peer selection nor mode click can connect after Down.
 action('mousemove','--window',wid,'470','510','click','1','mousemove','--window',wid,'620','510','click','1');time.sleep(.1);assert client.poll() is None and window()==wid,'down chooser initiated session'
 # ChannelTUN ownership is one-shot. Down disposes it, so recovery needs a
 # fresh actual process and fresh injected validation TUN, not fake re-Up.
 old=daemons['viewer-state'];old.terminate();assert old.wait(timeout=5)==0;processes.remove(old)
 daemons['viewer-state']=start([out/'mnetd','-state',viewer,'-fake-tun'],'viewer-restarted')
 poll(lambda:api(viewer)['status']=='down','fresh actual daemon')
 api(viewer,'up',{});poll(lambda:api(viewer)['status']=='up' and api(viewer,'session-peers'),'actual saved identity process recovery')
 action('key','r');phase('ready',3);capture('actual-daemon-up-recovered');assert 'detailsfalse' in title(),'Up revived old details'
 assert api(viewer,'session-peers')[0]['PublicKey']==host_identity['public_key'],'recovery changed peer key'
 for name,state in states.items():assert identities[name]==(state/'state.json').read_bytes(),'chooser/lifecycle changed durable identity'
 action('key','Escape');assert client.wait(timeout=5)==0;processes.remove(client)
 # A displayed selection cannot survive actual Down merely because the UI
 # has not refreshed. Both modes re-read real daemon evidence before dialing.
 for mode,x in [('view-only',470),('pointer',620)]:
  client=start([out/'mnet','--state',viewer,'session','browse'],'stale-'+mode,env)
  wid=poll(window,'stale '+mode+' chooser');action('windowfocus',wid);phase('ready',1)
  action('mousemove','--window',wid,'120','120','click','1');poll(lambda:capture('stale-'+mode+'-selected').getpixel((25,110))==(0,102,204),'actual selected '+mode+' pixels')
  api(viewer,'down',{});poll(lambda:api(viewer)['status']=='down','stale '+mode+' actual Down')
  assert not api(viewer,'session-peers'),'stale-mode Down retained authority'
  # No Refresh. The old row is deliberately still displayed and selected.
  action('mousemove','--window',wid,str(x),'510','click','1')
  assert client.wait(timeout=5)!=0,'stale selection accepted after actual Down'
  processes.remove(client)
  refusal=(out/('stale-'+mode+'.log')).read_text()
  assert 'network is not up' in refusal,'expected pre-dial actual-status refusal missing'
  assert 'dial tcp' not in refusal and not window(),'stale choice reached dial or retained chooser'
  old=daemons['viewer-state'];old.terminate();assert old.wait(timeout=5)==0;processes.remove(old)
  daemons['viewer-state']=start([out/'mnetd','-state',viewer,'-fake-tun'],'recovered-after-'+mode)
  poll(lambda:api(viewer)['status']=='down','new daemon after '+mode)
  api(viewer,'up',{});poll(lambda:api(viewer)['status']=='up' and api(viewer,'session-peers'),'recovery after '+mode)
  assert identities['viewer-state']==(viewer/'state.json').read_bytes(),'stale refusal changed durable identity'
  print('PASS actual selected '+mode+' peer, actual daemon Down with NO Refresh, explicit mode refused by actual status before OS dial; fresh-process recovery preserves identity',flush=True)
 print('PASS actual built control + two owned mnetd ChannelTUN processes + native chooser: actual Up/map-derived exact peer key, selected details, Down invalidation, cleared selection/modes, saved identity fresh-process Up recovery, unchanged State. No synthetic API or record injection. NO kernel TUN, OS mesh TCP, attended session, physical/native OS or release acceptance.',flush=True)
finally:
 for p in reversed(processes):
  if p.poll() is None:p.terminate()
 for p in reversed(processes):
  try:p.wait(timeout=5)
  except subprocess.TimeoutExpired:p.kill();p.wait()
 print('Artifacts retained: '+str(out),flush=True)
