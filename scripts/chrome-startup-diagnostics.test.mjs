import {test} from 'node:test';
import assert from 'node:assert/strict';
import {chromeStartupDiagnostics} from './chrome-startup-diagnostics.mjs';
test('startup diagnostics join chunks but disclose fixed categories only',()=>{
 const d=chromeStartupDiagnostics();assert.match(d.summary(),/stderrSeen=false/);
 d.feed('synthetic-private-path ProcessSingle');d.feed('ton profile in use');
 assert.match(d.summary(),/profileLockError=true/);assert.doesNotMatch(d.summary(),/synthetic|Process|profile in use/);
 d.feed('no usable sandbox');d.feed('FATAL: synthetic-private-message');d.feed('remote-debugging-pipe file descriptors are not open');
 assert.match(d.summary(),/sandboxError=true/);assert.match(d.summary(),/crashMarker=true/);assert.match(d.summary(),/debugPipeError=true/);
});
test('normal stderr does not imply startup failure and storage stays bounded',()=>{
 const d=chromeStartupDiagnostics();for(let i=0;i<500;i++)d.feed('unrelated synthetic diagnostic '.repeat(100));
 assert.equal(d.summary(),'stderrSeen=true sandboxError=false profileLockError=false crashMarker=false debugPipeError=false');
});
