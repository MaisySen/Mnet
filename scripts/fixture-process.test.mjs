import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {stopChild} from './fixture-process.mjs';
const start=async code=>{const child=spawn(process.execPath,['-e',code],{stdio:['ignore','pipe','ignore']});await new Promise(resolve=>child.stdout.once('data',resolve));return child;};
test('waits for child cleanup before returning',async()=>{
 const child=await start(`process.on('SIGTERM',()=>setTimeout(()=>process.exit(0),100));console.log('ready');setInterval(()=>{},1000);`);
 const before=Date.now();await stopChild(child);assert.ok(Date.now()-before>=80);assert.equal(child.exitCode,0);
 await stopChild(child);await stopChild(null);
});
test('escalates an unresponsive child within bounded time',async()=>{
 const child=await start(`process.on('SIGTERM',()=>{});console.log('ready');setInterval(()=>{},1000);`);
 await stopChild(child,100);assert.equal(child.signalCode,'SIGKILL');
});
