#!/usr/bin/env python3
"""Reject unsupported RGB control visuals before rendering or daemon reads."""
import json,os,pathlib,socketserver,subprocess,tempfile,threading,time
os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-controls-visual.'));print(out,flush=True)
executable=os.environ.get('MNET_CONTROLS_EXECUTABLE',str(out/'mnet'))
if 'MNET_CONTROLS_EXECUTABLE' not in os.environ:subprocess.run(['go','build','-race','-o',executable,'./cmd/mnet'],check=True)
subprocess.run(['go','build','-race','-o',str(out/'mnetd'),'./cmd/mnetd'],check=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'desk-fixture.test'),'./cmd/mnetd'],check=True)
state=out/'state';state.mkdir(mode=0o700)
class Handler(socketserver.BaseRequestHandler):
 def handle(self):self.server.calls+=1
server=socketserver.UnixStreamServer(str(state/'mnetd.sock'),Handler);server.calls=0
thread=threading.Thread(target=server.serve_forever);thread.start()
layout=out/'layout.json';layout.write_text(json.dumps(dict(version=1,displays=[dict(id='a',device='left',x=0,y=0,width=100,height=100,pixel_width=1000,pixel_height=1000),dict(id='b',device='right',x=100,y=0,width=100,height=100,pixel_width=1000,pixel_height=1000)])))
x=None;client=None
try:
 for depth in (8,16):
  displayfile=out/f'display{depth}'
  with displayfile.open('w') as fd,(out/f'x{depth}.log').open('w') as log:x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0',f'1024x768x{depth}'],pass_fds=(fd.fileno(),),stdout=log,stderr=log)
  deadline=time.monotonic()+5
  while not displayfile.read_text().strip():
   if x.poll() is not None or time.monotonic()>deadline:raise AssertionError('disposable display unavailable')
   time.sleep(.02)
  env=os.environ|{'DISPLAY':':'+displayfile.read_text().strip()};env.pop('WAYLAND_DISPLAY',None);env.pop('XDG_SESSION_TYPE',None)
  for mode,args in [('devices',['devices']),('chooser',['session','browse']),('layout',['desk','layout-view',str(layout)])]:
   path=out/f'{depth}-{mode}.log'
   with path.open('w') as log:client=subprocess.Popen([executable,'--state',str(state),*args],env=env,stdout=log,stderr=log)
   deadline=time.monotonic()+3
   while client.poll() is None and time.monotonic()<deadline:
    result=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet'],env=env,text=True,capture_output=True)
    if result.returncode==0 and result.stdout.strip():
     wid=result.stdout.strip().splitlines()[-1];subprocess.run(['import','-display',env['DISPLAY'],'-window',wid,str(out/f'{depth}-{mode}-unsupported.png')],check=True)
     raise AssertionError(f'{mode} rendered native controls on unsupported {depth}-bit visual')
    time.sleep(.02)
   assert client.poll() is not None,f'{mode} failed to retire unsupported visual'
   assert client.returncode!=0 and 'controls require supported 24-bit TrueColor X11' in path.read_text(),path.read_text()
   assert 'DATA RACE' not in path.read_text();client=None
  # The actual Desk launcher must reject unsupported RGB before local
  # review, a consumed replay token, a listener or any native owner exists.
  deskdir=out/f'desk{depth}';deskdir.mkdir()
  subprocess.run([str(out/'desk-fixture.test'),'-test.run=^TestDeskX11PrepareLauncher$','-test.timeout=10s'],env=env|{'MNET_DESK_LAUNCH_PREPARE':'1','MNET_DESK_TARGET_DISPLAY':env['DISPLAY'],'MNET_DESK_ARTIFACTS':str(deskdir)},check=True,capture_output=True,text=True)
  for role in ('primary','recipient'):
   path=out/f'{depth}-desk-{role}.log'
   with path.open('w') as log:client=subprocess.Popen([str(out/'mnetd'),'-desk-run',str(deskdir/role/'run.json')],env=env,stdout=log,stderr=log)
   try:client.wait(timeout=2)
   except subprocess.TimeoutExpired:
    result=subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet Shared Desk'],env=env,text=True,capture_output=True)
    if result.returncode==0 and result.stdout.strip():
     subprocess.run(['import','-display',env['DISPLAY'],'-window',result.stdout.strip().splitlines()[-1],str(out/f'{depth}-desk-{role}-unsupported.png')],check=True)
    raise AssertionError('Desk launcher did not refuse unsupported visual before review')
   assert client.returncode!=0 and 'supported 24-bit TrueColor X11' in path.read_text(),path.read_text()
   assert not list((deskdir/role/'runs').iterdir()),'unsupported Desk visual consumed run'
   assert not subprocess.run(['xdotool','search','--onlyvisible','--name','^Mnet Shared Desk'],env=env,capture_output=True).stdout.strip(),'unsupported Desk visual left window'
   client=None
  x.terminate();x.wait(timeout=3);x=None
 assert server.calls==0,'unsupported visual contacted daemon'
 print('PASS actual built devices/chooser/layout plus both Desk launcher roles:8-bit and16-bit root visuals rejected before native window, review, replay reservation or daemon calls. Supported24-bit actual journeys remain separate gates.',flush=True)
finally:
 if client is not None and client.poll() is None:client.terminate();client.wait(timeout=3)
 if x is not None:x.terminate();x.wait(timeout=3)
 server.shutdown();server.server_close();thread.join(timeout=3)
