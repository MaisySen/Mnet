// Isolated real-Chrome operator dashboard regression. No user profiles or live
// control service. Node 22+ and Chrome are required. Screenshots stay local.
import {spawn, spawnSync} from 'node:child_process';
import {mkdtempSync, writeFileSync, rmSync, existsSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:net';
import {setTimeout as delay} from 'node:timers/promises';
const dir=mkdtempSync(join(tmpdir(),'mnet-dashboard-'));
let control,chrome,ws;let sequence=0;const pending=new Map();
async function port(){const s=createServer();await new Promise(r=>s.listen(0,'127.0.0.1',r));const p=s.address().port;await new Promise(r=>s.close(r));return p;}
async function until(fn,description,budget=15000){const end=Date.now()+budget;let last;while(Date.now()<end){try{const value=await fn();if(value)return value;}catch(e){last=e;}await delay(50);}throw new Error('Timed out: '+description+(last?' '+last.message:''));}
async function cdp(method,params={}){const id=++sequence;const p=new Promise((resolve,reject)=>pending.set(id,{resolve,reject}));ws.send(JSON.stringify({id,method,params}));return p;}
async function js(expression){const r=await cdp('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(JSON.stringify(r.exceptionDetails));return r.result.value;}
async function shot(name){if(!process.env.MNET_DASHBOARD_SHOTS)return;const r=await cdp('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});writeFileSync(join(process.env.MNET_DASHBOARD_SHOTS,name+'.png'),Buffer.from(r.data,'base64'));}
try{
 const binary=join(dir,'mnet-control');
 const build=spawnSync('go',['build','-o',binary,'./cmd/mnet-control'],{stdio:'inherit'});if(build.status!==0)throw new Error('control build failed');
 const controlPort=await port(),debugPort=await port();const base='http://127.0.0.1:'+controlPort;
 control=spawn(binary,['-listen','127.0.0.1:'+controlPort,'-base-url',base,'-data',join(dir,'state.json'),'-default-network','fixture','-admin-password','local-browser-fixture'],{stdio:'ignore'});
 await until(async()=> (await fetch(base+'/healthz')).ok,'local control ready');
 const executable=process.env.CHROME_BIN||['/usr/bin/google-chrome','/usr/bin/chromium','/usr/bin/chromium-browser'].find(existsSync);if(!executable)throw new Error('Chrome missing');
 chrome=spawn(executable,['--headless','--no-sandbox','--disable-gpu','--disable-dev-shm-usage','--remote-debugging-port='+debugPort,'--user-data-dir='+join(dir,'chrome'),'--window-size=1280,1000','about:blank'],{stdio:['ignore','ignore','pipe']});
 let chromeErrors='';chrome.stderr.on('data',chunk=>{chromeErrors=(chromeErrors+chunk.toString()).slice(-4096);});
 let target;
 try{target=await until(async()=>{if(chrome.exitCode!==null)throw new Error('Chrome exited '+chrome.exitCode);const tabs=await(await fetch('http://127.0.0.1:'+debugPort+'/json')).json();return tabs.find(t=>t.type==='page');},'Chrome target',30000);}
 catch(error){throw new Error(error.message+'; Chrome startup diagnostics: '+chromeErrors);}
 ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j;});ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id&&pending.has(m.id)){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(new Error(m.error.message)):p.resolve(m.result);}};
 await cdp('Page.enable');await cdp('Runtime.enable');await cdp('Network.enable');
 await cdp('Page.navigate',{url:base+'/admin'});
 await until(()=>js(`document.querySelector('#login') && getComputedStyle(document.querySelector('#login')).display!=='none'`),'initial login visible');await shot('dashboard-login');
 await js(`document.querySelector('#pw').value='local-browser-fixture';document.querySelector('#loginBtn').click()`);
 await until(()=>js(`getComputedStyle(document.querySelector('#app')).display!=='none' && getComputedStyle(document.querySelector('#login')).display==='none'`),'signed-in app only');
 await until(()=>js(`document.querySelector('#nets tbody').textContent.includes('fixture')`),'real network rendered');if(!await js(`document.querySelector('#app').textContent.includes('Exit-node selection and internet routing are unsupported') && !document.querySelector('#app').textContent.includes('0.0.0.0/0 for exit-node internet access')`))throw new Error('ACL help implies unsupported exit routing');await shot('dashboard-signed-in');
 // Create an explicit private grant through the actual guarded operator API.
 await js(`(async()=>{window.memberNetwork=document.querySelector('#memberNet').value;await api('/admin/api/memberships',{network_id:memberNetwork,issuer:'https://issuer.example',subject:'<img src=x onerror=alert(1)>',allow:true});document.querySelector('#memberRead').click();})()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('<img src=x onerror=alert(1)>')`),'actual explicit grant rendered');
 if(!await js(`document.querySelectorAll('#members tbody tr').length===1 && !document.querySelector('#members tbody img')`))throw new Error('private grant did not render as text');
 await shot('dashboard-memberships');
 // Cancellation is not a write; a confirmed removal targets the listed tuple
 // through the real guarded API, and refreshing proves it disappeared.
 await js(`window.realConfirm=window.confirm;window.confirm=()=>false;document.querySelector('#members tbody button').click()`);
 if(!await js(`document.querySelectorAll('#members tbody tr').length===1`))throw new Error('cancelled revoke cleared grants');
 if(!await js(`(async()=>{const result=await api('/admin/api/memberships?network_id='+encodeURIComponent(memberNetwork));return result.memberships.length===1;})()`))throw new Error('cancelled revoke changed server membership');
 // An unconfirmed network outcome must not claim removal succeeded. Inspect
 // again reads the actual unchanged service before the real removal below.
 await js(`(async()=>{window.revokeFetch=window.fetch;window.confirm=()=>true;window.fetch=(input,options)=>input==='/admin/api/memberships' && options?.method==='POST'?Promise.reject(new Error('fixture interrupted response')):revokeFetch(input,options);document.querySelector('#members tbody button').click();})()`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent.startsWith('Could not confirm removal.') && !document.querySelector('#members tbody').textContent`),'honest unconfirmed removal state');
 await js(`window.fetch=revokeFetch;document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('<img')`),'actual inspection retry after unconfirmed removal');
 await js(`window.confirm=text=>{window.memberConfirmation=text;return true;};document.querySelector('#members tbody button').click()`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent==='No explicit grants for this network.'`),'actual confirmed membership removal');
 if(!await js(`memberConfirmation.includes('https://issuer.example') && memberConfirmation.includes('<img src=x onerror=alert(1)>') && memberConfirmation.includes('NOT disconnect VPN devices')`))throw new Error('removal confirmation omitted target/scope');
 await js(`(async()=>{window.confirm=realConfirm;await api('/admin/api/memberships',{network_id:memberNetwork,issuer:'https://issuer.example',subject:'<img src=x onerror=alert(1)>',allow:true});await loadMembers();})()`);
 // The POST can commit before its response is lost. Preserve uncertainty,
 // then let a fresh actual GET settle the result without resending the write.
 await js(`(async()=>{window.appliedFetch=window.fetch;window.confirm=()=>true;window.fetch=async(input,options)=>{if(input==='/admin/api/memberships' && options?.method==='POST'){const response=await appliedFetch(input,options);if(!response.ok)throw new Error('fixture write failed');throw new Error('fixture lost committed response');}return appliedFetch(input,options);};document.querySelector('#members tbody button').click();})()`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent.startsWith('Could not confirm removal.')`),'lost committed response kept uncertain');
 await js(`window.fetch=appliedFetch;window.confirm=realConfirm;document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent==='No explicit grants for this network.'`),'actual GET settled committed removal');
 await js(`(async()=>{await api('/admin/api/memberships',{network_id:memberNetwork,issuer:'https://issuer.example',subject:'<img src=x onerror=alert(1)>',allow:true});await loadMembers();window.oldRemove=document.querySelector('#members tbody button').onclick;window.confirm=()=>{throw new Error('stale row reached confirmation');};clearMembers();await oldRemove();window.confirm=realConfirm;await loadMembers();})()`);
 if(!await js(`document.querySelectorAll('#members tbody tr').length===1`))throw new Error('stale row callback changed membership');
 // Changing the selection while confirmation is open invalidates the action.
 await js(`(async()=>{window.confirm=()=>{clearMembers();return true;};document.querySelector('#members tbody button').click();window.confirm=realConfirm;await loadMembers();})()`);
 if(!await js(`document.querySelectorAll('#members tbody tr').length===1`))throw new Error('confirmation scope change still removed membership');
 // Browser-local edge responses test safe failure/retry presentation; initial
 // login, grant creation and the successful inspection above use the server.
 await js(`(async()=>{window.edgeFetch=window.fetch;window.fetch=(input,options)=>String(input).startsWith('/admin/api/memberships?')?Promise.reject(new Error('private transport details')):edgeFetch(input,options);await loadMembers();window.fetch=edgeFetch;})()`);
 if(!await js(`!document.querySelector('#members tbody').textContent && document.querySelector('#memberMsg').textContent==='Could not read grants. Try again.'`))throw new Error('failed membership read retained private rows or transport text');
 await js(`document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('<img')`),'actual retry after failed membership read');
 // Two reads of the same selection complete in reverse order. Only the most
 // recent can render, even if the earlier response was valid on arrival.
 await js(`(async()=>{window.reverseFetch=window.fetch;window.memberReleases=[];window.fetch=(input,options)=>String(input).startsWith('/admin/api/memberships?')?new Promise(resolve=>memberReleases.push(body=>resolve(new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})))):reverseFetch(input,options);window.firstMember=loadMembers();window.secondMember=loadMembers();memberReleases[1]({memberships:[{network_id:memberNetwork,issuer:'https://issuer.example',subject:'latest-request'}]});await secondMember;memberReleases[0]({memberships:[{network_id:memberNetwork,issuer:'https://issuer.example',subject:'stale-request'}]});await firstMember;window.fetch=reverseFetch;})()`);
 if(!await js(`document.querySelectorAll('#members tbody tr').length===1 && document.querySelector('#members tbody').textContent.includes('latest-request') && !document.querySelector('#members tbody').textContent.includes('stale-request')`))throw new Error('old same-network read replaced a newer result');
 for(const body of ['{invalid JSON','{}','null',JSON.stringify({memberships:[null]}),JSON.stringify({memberships:[{}]}),JSON.stringify({memberships:[{network_id:'wrong-network',issuer:'https://issuer.example',subject:'private-outsider'}]})]){
  await js(`(async()=>{window.badFetch=window.fetch;window.fetch=(input,options)=>String(input).startsWith('/admin/api/memberships?')?Promise.resolve(new Response(${JSON.stringify(body)},{status:200,headers:{'Content-Type':'application/json'}})):badFetch(input,options);await loadMembers();window.fetch=badFetch;})()`);
  if(!await js(`!document.querySelector('#members tbody').textContent && document.querySelector('#memberMsg').textContent==='Could not read grants. Try again.'`))throw new Error('invalid successful membership response was treated as grants');
 }
 // A second network has no grants. Switching clears old private rows immediately.
 await js(`(async()=>{window.otherNetwork=await api('/admin/api/networks',{name:'empty-membership-network'});await load();document.querySelector('#memberNet').value=otherNetwork.id;document.querySelector('#memberNet').dispatchEvent(new Event('change'));})()`);
 if(!await js(`!document.querySelector('#members tbody').textContent`))throw new Error('network switch retained previous grants');
 await js(`document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent==='No explicit grants for this network.'`),'actual empty network grants');
 // Hold a valid result across a network switch. It must not repaint the old network.
 await js(`(async()=>{document.querySelector('#memberNet').value=memberNetwork;document.querySelector('#memberNet').dispatchEvent(new Event('change'));window.memberSavedFetch=window.fetch;window.memberBody=await (await fetch('/admin/api/memberships?network_id='+encodeURIComponent(memberNetwork))).text();window.fetch=(input,options)=>String(input).startsWith('/admin/api/memberships?')?new Promise(resolve=>{window.releaseMember=()=>resolve(new Response(memberBody,{status:200,headers:{'Content-Type':'application/json'}}));}):memberSavedFetch(input,options);window.memberPending=loadMembers();document.querySelector('#memberNet').value=otherNetwork.id;document.querySelector('#memberNet').dispatchEvent(new Event('change'));window.releaseMember();await memberPending;window.fetch=memberSavedFetch;})()`);
 if(!await js(`!document.querySelector('#members tbody').textContent && !document.querySelector('#memberMsg').textContent`))throw new Error('late result crossed network selection');
 await js(`document.querySelector('#memberNet').value=memberNetwork;document.querySelector('#memberNet').dispatchEvent(new Event('change'));document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('<img')`),'membership restored before logout');
 // Exercise actual key creation without recording or printing its credential.
 await js(`window.prompt=()=>null;document.querySelector('#keyNote').value='cleanup-fixture';document.querySelector('#keyTTL').value='1';document.querySelector('#newKey').requestSubmit()`);
 await until(()=>js(`document.querySelector('#keys tbody').textContent.includes('cleanup-fixture') && /[0-9]+[ms] left/.test(document.querySelector('#keys tbody').textContent)`),'populated unexpired key row');
 if(process.env.MNET_DASHBOARD_SHOTS){
  await js(`document.querySelectorAll('#keys .key code').forEach(cell=>cell.textContent='[fixture credential hidden]')`);
  await shot('dashboard-expiry');
 }

 if(!await js(`(()=>{const now=Date.now();return expires('',now)==='never' && expires('invalid',now)==='unknown' && expires(new Date(now).toISOString(),now)==='expired' && expires(new Date(now-1000).toISOString(),now)==='expired' && expires(new Date(now+30000).toISOString(),now)==='30s left' && expires(new Date(now+120000).toISOString(),now)==='2m left' && expires(new Date(now+7200000).toISOString(),now)==='2h left' && expires(new Date(now+172800000).toISOString(),now)==='2d left';})()`))throw new Error('expiry formatting boundary failed');
 if(!await js(`document.querySelector('#nodes thead').textContent.includes('Relay connection')`))throw new Error('presence label implies whole-device health');
 const cookies=(await cdp('Network.getCookies',{urls:[base]})).cookies;const token=cookies.find(c=>c.name==='mnet_admin')?.value;if(!token)throw new Error('session cookie missing');
 // Hold a valid membership response across logout as well as the state refresh.
 await js(`(async()=>{window.memberLogoutFetch=window.fetch;window.fetch=(input,options)=>String(input).startsWith('/admin/api/memberships?')?new Promise(resolve=>{window.releaseMemberLogout=()=>resolve(new Response(memberBody,{status:200,headers:{'Content-Type':'application/json'}}));}):memberLogoutFetch(input,options);window.memberLogoutPending=loadMembers();})()`);
 // Capture a valid old state response and hold a refresh after logout starts.
 await js(`(async()=>{window.savedFetch=window.fetch;window.heldResponse=await (await savedFetch('/admin/api/state')).text();window.releaseHeld=null;window.fetch=(input,options)=>input==='/admin/api/state'?new Promise(resolve=>{window.releaseHeld=()=>resolve(new Response(heldResponse,{status:200,headers:{'Content-Type':'application/json'}}));}):savedFetch(input,options);window.pendingRefresh=load();})()`);
 await js(`document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#loginMsg').textContent==='Signed out.'`),'logout complete');
 await js(`(async()=>{window.releaseHeld();await window.pendingRefresh;window.fetch=window.savedFetch;window.releaseMemberLogout();await window.memberLogoutPending;window.fetch=memberLogoutFetch;})()`);
 if(!await js(`state===null && getComputedStyle(document.querySelector('#app')).display==='none' && getComputedStyle(document.querySelector('#logout')).display==='none' && !document.querySelector('#nets tbody').textContent && !document.querySelector('#keys tbody').textContent && !document.querySelector('#keyNet').textContent && !document.querySelector('#memberNet').textContent && !document.querySelector('#members tbody').textContent && !document.querySelector('#memberMsg').textContent`))throw new Error('late refresh restored private dashboard state');
 const replay=await fetch(base+'/admin/api/state',{headers:{Cookie:'mnet_admin='+token}});if(replay.status!==401)throw new Error('logged-out cookie replay accepted');
 const remaining=(await cdp('Network.getCookies',{urls:[base]})).cookies;if(remaining.some(c=>c.name==='mnet_admin'))throw new Error('logout cookie not cleared');
 await shot('dashboard-signed-out');
 console.log('PASS real Chrome dashboard: login visibility, same-origin sign-in, network/key render, actual scoped membership reads, cancel/confirmed removal and refresh, lost-committed-response inspection, stale row/confirmation guard, HTML-as-text, empty grants, failed-read clear/retry, invalid-response handling, same-network out-of-order reads, network-switch/logout late-result suppression, expiry boundaries, sign-out, cleared populated private UI, delayed-refresh suppression and logged-out token replay');
}finally{ws?.close();chrome?.kill();control?.kill();await delay(150);rmSync(dir,{recursive:true,force:true});}
