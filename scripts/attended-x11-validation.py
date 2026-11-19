#!/usr/bin/env python3
"""Synthetic attended-host proof on an owned disposable X display only."""
import subprocess,os,time,pathlib,tempfile,shutil,signal
def action_point(width,height,stage):
 return width-{'pause':225,'revoke':75}[stage],round(height*.86)

# Exact observed button rectangles from retained local GTK3 and hosted GTK4
# pixels. The old bottom-edge point is outside the hosted Pause rectangle.
for width,height,pause,revoke in [(564,225,(276,185,413,218),(419,185,557,218)),(606,341,(216,268,391,312),(403,268,577,312))]:
 for stage,rect in [('pause',pause),('revoke',revoke)]:
  px,py=action_point(width,height,stage)
  assert rect[0]<px<rect[2] and rect[1]<py<rect[3], 'fixture action misses inspected native button'
assert not (268<341-25<312), 'original hosted mismatch no longer represented'

os.chdir(pathlib.Path(__file__).resolve().parent.parent)
for command in ['go','Xvfb','xdotool','import','xwininfo']:
 if shutil.which(command) is None:raise SystemExit('Missing local fixture dependency: '+command)

out=pathlib.Path(tempfile.mkdtemp(prefix='mnet-attended-x11.'))
print('Attended X11 artifacts: '+str(out),flush=True)
subprocess.run(['go','test','-race','-c','-o',str(out/'attended.test'),'./cmd/mnetd'],check=True)
(out/'stage').unlink(missing_ok=True)
with open(out/'display','w') as fd,open(out/'xvfb.log','w') as log:
 x=subprocess.Popen(['Xvfb','-noreset','-nolisten','tcp','-displayfd',str(fd.fileno()),'-screen','0','1024x768x24'],stdout=log,stderr=log,pass_fds=(fd.fileno(),))
test=None
try:
 for _ in range(100):
  if (out/'display').read_text().strip():break
  if x.poll() is not None:raise AssertionError('Disposable display failed to start')
  time.sleep(.05)
 number=(out/'display').read_text().strip()
 if not number.isdecimal() or x.poll() is not None:raise AssertionError('No owned disposable display identifier')
 (out/'Xauthority').touch(mode=0o600)
 env=os.environ|{'DISPLAY':':'+number,'XAUTHORITY':str(out/'Xauthority'),'NO_AT_BRIDGE':'1','MNET_ATTENDED_X11_TEST':'1','MNET_ATTENDED_ARTIFACTS':str(out)}
 env.pop('WAYLAND_DISPLAY',None)
 root_info=subprocess.check_output(['xwininfo','-root'],env=env,text=True)
 root=root_info.split('Window id: ',1)[1].split()[0]
 with open(out/'safety.log','w') as safety_log:
  subprocess.run([str(out/'attended.test'),'-test.run','^TestAttendedX11Safety','-test.v','-test.timeout=15s'],env=env,stdout=safety_log,stderr=safety_log,check=True)
 with open(out/'test.log','w') as log:
  test=subprocess.Popen([str(out/'attended.test'),'-test.run','^TestAttendedX11RealLocalSurface$','-test.v','-test.timeout=30s'],env=env,stdout=log,stderr=log,start_new_session=True)
  seen=set();start=time.monotonic()
  def window(title):
   p=subprocess.run(['xdotool','search','--onlyvisible','--name',title],env=env,text=True,capture_output=True)
   return p.stdout.strip().splitlines()[-1] if p.returncode==0 and p.stdout.strip() else None
  def screenshot(wid,name):
   time.sleep(.12);subprocess.run(['import','-window',wid,str(out/(name+'.png'))],env=env,check=True)
  def allow(wid):subprocess.run(['xdotool','windowfocus',wid,'key','Tab','Return'],env=env,check=True)
  while test.poll() is None and time.monotonic()-start<29:
   stage=(out/'stage').read_text() if (out/'stage').exists() else ''
   if stage in seen:time.sleep(.01);continue
   if stage=='view':
    wid=window('^Mnet: allow viewing');
    if wid:screenshot(wid,'view');allow(wid);seen.add(stage)
   elif stage=='active':
    wid=window('^Mnet: desktop sharing');
    if wid:screenshot(wid,'active');seen.add(stage);(out/'driver-ack').write_text('active')
   elif stage in ('control','resume','control-again'):
    wid=window('^Mnet: allow control');
    if wid:screenshot(wid,stage);allow(wid);seen.add(stage)
   elif stage in ('pause','revoke','global-stop-held'):
    wid=window('^Mnet: desktop sharing');
    if wid:
     geo=subprocess.check_output(['xdotool','getwindowgeometry','--shell',wid],env=env,text=True)
     d=dict(line.split('=',1) for line in geo.splitlines() if '=' in line);w=int(d['WIDTH']);h=int(d['HEIGHT'])
     screenshot(wid,stage)
     # Inspect retained pixels too: current native info layout is End, Pause, Revoke.
     # The lower 86% row lies inside both inspected GTK3/GTK4 button bands;
     # h-25 missed the taller GTK4 buttons. State assertions remain authoritative.
     if stage=='global-stop-held':
      # Focus another mapped window, not any Mnet dialog, before the shortcut.
      foreign=(out/'reclaim-window').read_text().strip()
      subprocess.run(['xdotool','windowfocus','--sync',foreign],env=env,check=True)
      focus=subprocess.check_output(['xdotool','getwindowfocus'],env=env,text=True).strip()
      assert int(focus,0)==int(foreign,0), 'stop fixture did not move focus away from Mnet'
      screenshot(root,'foreign-focus-held')
      # Extra lock state must not bypass the reserved chord.
      subprocess.run(['xdotool','key','Caps_Lock'],env=env,check=True)
      subprocess.run(['xdotool','key','ctrl+alt+shift+F12'],env=env,check=True)
     else:
      px,py=action_point(w,h,stage)
      subprocess.run(['xdotool','windowraise',wid,'mousemove','--sync','--window',wid,str(px),str(py),'click','1'],env=env,check=True)
     seen.add(stage)
   time.sleep(.02)
  result=test.wait(timeout=3)
  expected={'view','active','control','pause','resume','revoke','control-again','global-stop-held'}
  print('Seen fixture stages: '+','.join(sorted(seen)),flush=True)
  if result!=0 or seen!=expected:raise AssertionError('Attended runtime proof failed; inspect retained test.log and pixels')
  print('PASS: actual local view/control, pause/resume/revoke, focus-independent live stop with held XTEST input, and joined teardown',flush=True)

finally:
 if test:
  try:os.killpg(test.pid,signal.SIGKILL)
  except ProcessLookupError:pass
  test.wait()
 x.terminate();x.wait()
