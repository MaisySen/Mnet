import {test} from 'node:test';
import assert from 'node:assert/strict';
import {chromeWaitCategory,chromeWaitSnapshot} from './chrome-wait-diagnostics.mjs';
test('wait categories are fixed, exact and never disclose kernel/private text',()=>{
 for(const [raw,want] of [['0\n','not-blocked-or-hidden'],['futex_wait_queue','futex'],['do_epoll_wait','poll'],['anon_pipe_read','pipe'],['hrtimer_nanosleep','sleep'],['folio_wait_bit_common','io-wait'],['blk_mq_get_tag','io-wait'],['private-secret-io_schedule','other-or-unavailable'],['io_schedule private-secret','other-or-unavailable'],['','other-or-unavailable']])assert.equal(chromeWaitCategory(raw),want);
});
test('snapshot follows only bounded numeric direct children and emits only counts',()=>{
 const calls=[];
 const files={'/proc/123/wchan':'io_schedule','/proc/123/task/123/children':'456 789 999','/proc/456/wchan':'futex_wait_queue','/proc/789/wchan':'private-secret-kernel-symbol'};
 const summary=chromeWaitSnapshot(123,(path,limit)=>{calls.push([path,limit]);if(!(path in files))throw Error('private-secret-error');return files[path];});
 assert.match(summary,/mainWait=io-wait childWaitSamples=3 childWaitUnavailable=1 childWaitIO=0 childWaitFutex=1/);
 assert.match(summary,/childWaitOther=2 childWaitTruncated=false/);
 assert.doesNotMatch(summary,/private|secret|123|456|789|999|proc|schedule|queue/);
 assert.ok(calls.every(([,limit])=>limit<=4096));
});
test('invalid, oversized or changing proc entries fail closed without new reads',()=>{
 for(const children of ['456 private-secret','../private','123/../../private','1'.repeat(4097)]){
  const calls=[];const summary=chromeWaitSnapshot(123,(path)=>{calls.push(path);return path.endsWith('/children')?children:'x'.repeat(257);});
  assert.match(summary,/mainWait=other-or-unavailable childWaitSamples=0/);assert.equal(calls.length,2);assert.doesNotMatch(summary,/private|secret|xxx/);
 }
 let calls=0;chromeWaitSnapshot(-1,()=>{calls++;});assert.equal(calls,0);
});
test('more than32 children are explicitly truncated and sampled at most32',()=>{
 let reads=0;const summary=chromeWaitSnapshot(123,(path)=>{reads++;return path.endsWith('/children')?Array.from({length:50},(_,i)=>i+1).join(' '):'0';});
 assert.match(summary,/childWaitSamples=32/);assert.match(summary,/childWaitHidden=32/);assert.match(summary,/childWaitTruncated=true/);assert.equal(reads,34);
});
test('actual local process snapshot is sanitized without requiring a live browser',async()=>{
 const {openSync,readSync,closeSync}=await import('node:fs');
 const summary=chromeWaitSnapshot(process.pid,(path,limit)=>{const fd=openSync(path,'r');try{const b=Buffer.alloc(limit+1);return b.subarray(0,readSync(fd,b,0,limit+1,null)).toString('utf8');}finally{closeSync(fd);}});
 assert.match(summary,/^mainWait=(?:not-blocked-or-hidden|futex|poll|pipe|sleep|io-wait|other-or-unavailable) childWaitSamples=\d+ /);
 assert.doesNotMatch(summary,/\/|private|secret/);
});
