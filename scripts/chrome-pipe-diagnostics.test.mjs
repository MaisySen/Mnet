import {test} from 'node:test';
import assert from 'node:assert/strict';
import {chromePipeDiagnostics,chromeVersionCategory,chromeProcessState} from './chrome-pipe-diagnostics.mjs';
test('pipe startup telemetry tracks callbacks without retaining payloads',()=>{
 let clock=100;const d=chromePipeDiagnostics(()=>clock);
 d.spawned();d.writing();d.wrote();clock=300;d.received('synthetic-secret');d.received('more-private-data');d.writing();d.wrote(Error('secret-error'));
 assert.match(d.summary(),/spawnSeen=true requestWrites=2 writeCallbacks=1 writeErrors=1 receiveBytes=33 firstResponse=lt1s/);
 assert.doesNotMatch(d.summary(),/synthetic|secret|private|error-message/);
});
test('silent, ended and closed transports remain distinguishable',()=>{
 const d=chromePipeDiagnostics();d.writing();d.sendEnded();d.receiveEnded();d.sendClosed();d.receiveClosed();
 assert.match(d.summary(),/writeCallbacks=0 writeErrors=0 receiveBytes=0 firstResponse=none/);
 assert.match(d.summary(),/sendEnded=true receiveEnded=true sendClosed=true receiveClosed=true/);
});
test('version output accepts only the known numeric product line',()=>{
 assert.equal(chromeVersionCategory('Google Chrome 156.0.8078.47\n'),'156.0.8078.47');
 assert.equal(chromeVersionCategory('Google Chrome for Testing 156.0.8078.47'),'156.0.8078.47');
 for(const text of ['secret','Google Chrome 156.0.8078.47 private','private\nGoogle Chrome 156.0.8078.47','156.0.8078.47'])assert.equal(chromeVersionCategory(text),'unavailable');
});
test('process stat returns only a fixed category even with private comm',()=>{
 assert.equal(chromeProcessState('123 (private (fixture)) S 5 6 7'),'sleeping');
 assert.equal(chromeProcessState('123 (private) T 5 6'),'stopped');
 assert.equal(chromeProcessState('private malformed'),'unavailable');
});
test('real inherited descriptors record write, response and peer closure',async()=>{
 const {spawn}=await import('node:child_process');
 const child=spawn(process.execPath,['-e',`const fs=require('node:fs');const input=fs.createReadStream(null,{fd:3});input.once('data',()=>{fs.writeSync(4,Buffer.from([123,125,0]));input.destroy();});`],{stdio:['ignore','ignore','ignore','pipe','pipe']});
 const d=chromePipeDiagnostics();
 child.once('spawn',()=>d.spawned());
 child.stdio[4].on('data',data=>d.received(data));
 child.stdio[3].once('close',()=>d.sendClosed());
 child.stdio[4].once('end',()=>d.receiveEnded());
 child.stdio[4].once('close',()=>d.receiveClosed());
 const closed=new Promise((resolve,reject)=>{child.once('error',reject);child.once('close',resolve);});
 d.writing();child.stdio[3].write(Buffer.from([123,125,0]),error=>d.wrote(error));
 const timer=setTimeout(()=>child.kill('SIGKILL'),3000);
 try{await closed;}finally{clearTimeout(timer);child.kill('SIGKILL');}
 assert.match(d.summary(),/spawnSeen=true requestWrites=1 writeCallbacks=1 writeErrors=0 receiveBytes=3/);
 assert.match(d.summary(),/receiveEnded=true sendClosed=true receiveClosed=true/);
});
test('first-response timing stays in fixed buckets and never changes afterward',()=>{
 for(const [elapsed,bucket] of [[0,'lt100ms'],[100,'lt1s'],[1000,'lt5s'],[5000,'lt15s'],[15000,'gte15s']]){
  let clock=0;const d=chromePipeDiagnostics(()=>clock);clock=elapsed;d.received('a');clock=99999;d.received('b');
  assert.match(d.summary(),new RegExp('firstResponse='+bucket+' '));
 }
});
