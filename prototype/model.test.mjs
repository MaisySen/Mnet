import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtures, initial, step } from './model.mjs';
const walk=(s,...actions)=>actions.reduce(step,s);
const enrolled = scenario => walk(initial(scenario),'SIGN_IN','RETURN','CHOOSE','ADD','ALLOW','CHECK','VERIFY','DONE');
const active = scenario => walk(enrolled(scenario),'DETAIL','CONNECT','REQUEST','HOST_REPLY','VIEW');

test('owner setup cannot report ready before both DNS and peer traffic',()=>{
 let s=walk(initial(),'SIGN_IN','RETURN','CHOOSE','ADD','ALLOW','CHECK');
 assert.equal(s.screen,'checks'); assert.equal(s.enrolled,false); assert.equal(s.traffic,false);
 s=step(s,'VERIFY'); assert.equal(s.screen,'ready'); assert.equal(s.dns,true); assert.equal(s.traffic,true);
});
test('identity-only login cannot enter an eligible workspace',()=>{
 const s=walk(initial('Not eligible'),'SIGN_IN','RETURN','CHOOSE','ADD','ALLOW','CHECK','VERIFY');
 assert.equal(s.screen,'blocked'); assert.equal(s.enrolled,false); assert.equal(s.session,false);
});
test('declined OS permission stays cancellable without connecting',()=>{
 let s=walk(initial('Permission declined'),'SIGN_IN','RETURN','CHOOSE','ADD','ALLOW');
 assert.equal(s.screen,'permission'); assert.equal(s.vpn,false); assert.equal(s.enrolled,false);
 s=step(s,'CANCEL'); assert.equal(s.screen,'devices'); assert.equal(s.vpn,false);
});
test('DNS failure cannot report connected or retain a VPN',()=>{
 const s=enrolled('Connection failed'); assert.equal(s.screen,'failed'); assert.equal(s.enrolled,false); assert.equal(s.vpn,false);
});
test('view consent and control consent are distinct',()=>{
 let s=active(); assert.equal(s.screen,'active'); assert.equal(s.capture,true); assert.equal(s.control,false);
 s=step(s,'HOST_ALLOW'); assert.equal(s.control,false);
 s=walk(s,'CONTROL_REQUEST','HOST_ALLOW'); assert.equal(s.control,true);
});
test('cancelled request ignores a late approval',()=>{
 const s=walk(enrolled(),'DETAIL','CONNECT','REQUEST','CANCEL','HOST_REPLY','VIEW','HOST_ALLOW');
 assert.equal(s.session,false); assert.equal(s.control,false); assert.equal(s.screen,'device');
});
test('network loss clears control and safe resume does not silently restore it',()=>{
 const s=walk(active(),'CONTROL_REQUEST','HOST_ALLOW','NETWORK_LOST','RECOVER');
 assert.equal(s.screen,'active'); assert.equal(s.control,false); assert.equal(s.session,true);
});
test('network loss during consent requires reconnect before host can allow',()=>{
 const s=walk(active(),'CONTROL_REQUEST','NETWORK_LOST','HOST_ALLOW');
 assert.equal(s.screen,'reconnecting'); assert.equal(s.control,false);
});
test('membership removal ends capture, control and session, including stale approval',()=>{
 const s=walk(active(),'CONTROL_REQUEST','HOST_ALLOW','MEMBERSHIP_REMOVED','HOST_ALLOW','RECOVER');
 assert.equal(s.screen,'blocked'); assert.equal(s.control,false); assert.equal(s.capture,false); assert.equal(s.session,false);
});
test('end and sign out stop the session',()=>{
 for(const a of ['END','SIGN_OUT']) {const s=step(active(),a); assert.equal(s.session,false);assert.equal(s.control,false);assert.equal(s.capture,false);}
});
test('offline and declined cases grant no session',()=>{
 let s=walk(initial('Peer offline'),'DETAIL','CONNECT','REQUEST','HOST_REPLY','VIEW');
 assert.equal(s.screen,'offline');assert.equal(s.session,false);
 s=active('Session declined'); assert.equal(s.screen,'declined'); assert.equal(s.session,false);
});
test('device removal has exact confirmation and cannot be replayed through stale detail',()=>{
 let s=walk(enrolled(),'DETAIL','REMOVE','CONFIRM_REMOVE','DETAIL','CONNECT','REQUEST','HOST_REPLY','VIEW');
 assert.equal(s.removed,true);assert.equal(s.session,false);assert.equal(s.screen,'devices');
 s=walk(enrolled(),'DETAIL','REMOVE','CANCEL');assert.equal(s.removed,false);assert.equal(s.screen,'device');
});
test('navigation cannot abandon setup or an active session',()=>{
 for(const s of [walk(initial(),'SIGN_IN'),walk(initial(),'SIGN_IN','RETURN','CHOOSE','ADD'),active()]) {
  assert.equal(step(s,'NAV_DEVICES').screen,s.screen);assert.equal(step(s,'NAV_SETTINGS').screen,s.screen);
 }
});
test('unknown events cannot grant permission',()=>{const s=initial();assert.deepEqual(step(s,'FORGED_ALLOW'),s);assert.throws(()=>initial('unknown'));});

test('all reachable prototype states preserve capability and eligibility invariants',()=>{
 const events=['SIGN_IN','RETURN','CHOOSE','ADD','ALLOW','DECLINE','CANCEL','CHECK','VERIFY','DONE','RETRY','DETAIL','CONNECT','REQUEST','HOST_REPLY','VIEW','CONTROL_REQUEST','HOST_ALLOW','NETWORK_LOST','RECOVER','TIMEOUT','END','REMOVE','CONFIRM_REMOVE','SETTINGS','DISCONNECT','BACK','SIGN_OUT','MEMBERSHIP_REMOVED','NAV_DEVICES','NAV_SETTINGS'];
 let count=0;
 for(const fixture of fixtures){
  const q=[initial(fixture)],seen=new Set();
  while(q.length){const s=q.shift();const key=JSON.stringify({...s,notice:''});if(seen.has(key))continue;seen.add(key);count++;
   assert.ok(!s.control || s.session && s.capture,fixture+': control requires capture and session');
   assert.ok(!s.capture || s.session,fixture+': capture requires session');
   assert.ok(s.screen!=='blocked' || !s.session && !s.control && !s.capture);
   assert.ok(s.screen!=='ready' || s.dns && s.traffic && s.enrolled);
   for(const e of events)q.push(step(s,e));
  }
 }
 assert.ok(count>100); console.log('Checked reachable model states:',count);
});
