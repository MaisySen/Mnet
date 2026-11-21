from pathlib import Path
import os,subprocess,time,json,hashlib
root=Path('/downloads/mnet-os-s11-fourth-readonly-evidence');root.mkdir(exist_ok=False)
binary=Path('/tmp/bootstrap-readonly-S11-COMPILED-NOT-RUN');expected='5c39bc3bfb48910e5facb2159059491d374143f840147f6d83f16668fae7ef34'
def ns():
 s=os.stat('/proc/self/ns/net');return {'device':s.st_dev,'inode':s.st_ino}
def identity(pid):
 try:
  path=Path('/proc')/str(pid);st=(path/'stat').read_text();a=st[st.rindex(')')+1:].split();exe=os.readlink(path/'exe');return {'pid':pid,'ppid':int(a[1]),'start_ticks':int(a[19]),'state':a[0],'exe':exe}
 except (FileNotFoundError,PermissionError,ProcessLookupError,ValueError):return None
def children(pid):
 try:return [int(x) for x in Path(f'/proc/{pid}/task/{pid}/children').read_text().split()]
 except (FileNotFoundError,PermissionError,ProcessLookupError):return []
def snapshot(driver):
 found=[];queue=[driver];seen=set()
 while queue:
  pid=queue.pop(0)
  if pid in seen:continue
  seen.add(pid);r=identity(pid)
  if r:found.append(r);queue.extend(children(pid))
 return found
assert os.getuid()!=0
assert hashlib.sha256(binary.read_bytes()).hexdigest()==expected
assert subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()=='7fdec1c851359aaabc2694ffd1d840858b3805ef'
meta={'version':1,'source':'7fdec1c851359aaabc2694ffd1d840858b3805ef','binary_sha256':expected,'outer_uid':os.getuid(),'wrapper':identity(os.getpid()),'outer_initial':ns(),'invocations':1};observed={};samples=[];start=time.monotonic_ns()
with (root/'stdout.json').open('wb') as out,(root/'stderr.txt').open('wb') as err:
 p=subprocess.Popen([str(binary),'--bootstrap-readonly'],stdout=out,stderr=err)
 meta['driver_pid']=p.pid;meta['driver_initial']=identity(p.pid)
 # Do not reap until the final owned-tree sample; a zombie preserves PID.
 deadline=time.monotonic()+10
 while True:
  recs=snapshot(p.pid);samples.append({'elapsed_ms':(time.monotonic_ns()-start)/1e6,'processes':recs})
  for r in recs:observed[(r['pid'],r['start_ticks'])]=r
  driver=next((r for r in recs if r['pid']==p.pid),None)
  if driver is None or driver['state']=='Z':break
  if time.monotonic()>deadline:
   meta['external_timeout']=True;p.kill();break
  time.sleep(.001)
 meta['last_before_reap']=snapshot(p.pid);meta['exit_code']=p.wait(timeout=2);meta['driver_reaped']=True
meta['elapsed_ms']=(time.monotonic_ns()-start)/1e6;meta['outer_final']=ns();meta['outer_unchanged']=meta['outer_initial']==meta['outer_final']
raw=(root/'stdout.json').read_text()
try:
 result=json.loads(raw);v=result.get('verdict',{})
 for item in [v.get('lifecycle'),v.get('failure'),v.get('bootstrap',{}).get('client',{}).get('lifecycle')]:
  if item and item.get('pid') and item.get('start_ticks'):observed.setdefault((item['pid'],item['start_ticks']),{'pid':item['pid'],'start_ticks':item['start_ticks'],'source':'bounded-verdict'})
except (ValueError,AttributeError):pass
survivors=[];post=[]
for key,old in observed.items():
 current=identity(old['pid']);same=current is not None and current['start_ticks']==old['start_ticks'];post.append({'recorded':old,'current':current,'same_lifetime':same})
 if same:survivors.append(current)
meta['recorded_lifetimes']=list(observed.values());meta['post_exact_lifetime_checks']=post;meta['exact_lifetime_survivors']=survivors;meta['tree_sampling_note']='1ms owned-tree snapshots may miss short-lived children; bounded verdict identities supplement. No name cleanup or signals except timeout safeguard if it fired.'
(root/'outer-verdict.json').write_text(json.dumps(meta,indent=2)+'\n');(root/'owned-tree-samples.json').write_text(json.dumps(samples,indent=2)+'\n')
print(json.dumps(meta,indent=2));print('STDOUT\n'+raw);print('STDERR\n'+(root/'stderr.txt').read_text())