// Isolated real-Chrome operator dashboard regression. No user profiles or live
// control service. Node 22+ and Chrome are required. Screenshots stay local.
import {spawn, spawnSync} from 'node:child_process';
import {mkdtempSync, writeFileSync, rmSync, existsSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createServer} from 'node:net';
import {setTimeout as delay} from 'node:timers/promises';
const dir=mkdtempSync(join(tmpdir(),'mnet-account-'));
let chrome,ws;let sequence=0;const pending=new Map();
async function port(){const s=createServer();await new Promise(r=>s.listen(0,'127.0.0.1',r));const p=s.address().port;await new Promise(r=>s.close(r));return p;}
async function until(fn,description,budget=15000){const end=Date.now()+budget;let last;while(Date.now()<end){try{const value=await fn();if(value)return value;}catch(e){last=e;}await delay(50);}throw new Error('Timed out: '+description+(last?' '+last.message:''));}
async function cdp(method,params={}){const id=++sequence;const p=new Promise((resolve,reject)=>pending.set(id,{resolve,reject}));ws.send(JSON.stringify({id,method,params}));return p;}
async function js(expression){const r=await cdp('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(JSON.stringify(r.exceptionDetails));return r.result.value;}
async function shot(name){if(!process.env.MNET_ACCOUNT_SHOTS)return;const r=await cdp('Page.captureScreenshot',{format:'png',captureBeyondViewport:true});writeFileSync(join(process.env.MNET_ACCOUNT_SHOTS,name+'.png'),Buffer.from(r.data,'base64'));}
try{
 const base=process.env.MNET_ACCOUNT_BASE,network=process.env.MNET_ACCOUNT_NETWORK;
 if(!base||!network)throw new Error('isolated fixture missing');
 const debugPort=await port();
 const executable=process.env.CHROME_BIN||['/usr/bin/google-chrome','/usr/bin/chromium','/usr/bin/chromium-browser'].find(existsSync);if(!executable)throw new Error('Chrome missing');
 chrome=spawn(executable,['--headless','--no-sandbox','--disable-gpu','--disable-dev-shm-usage','--remote-debugging-port='+debugPort,'--user-data-dir='+join(dir,'chrome'),'--window-size=1280,1000','about:blank'],{stdio:['ignore','ignore','pipe']});
 let chromeErrors='';chrome.stderr.on('data',chunk=>{chromeErrors=(chromeErrors+chunk.toString()).slice(-4096);});
 let target;
 try{target=await until(async()=>{if(chrome.exitCode!==null)throw new Error('Chrome exited '+chrome.exitCode);const tabs=await(await fetch('http://127.0.0.1:'+debugPort+'/json')).json();return tabs.find(t=>t.type==='page');},'Chrome target',30000);}
 catch(error){throw new Error(error.message+'; Chrome startup diagnostics: '+chromeErrors);}
 ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j;});ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id&&pending.has(m.id)){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(new Error(m.error.message)):p.resolve(m.result);}};
 await cdp('Page.enable');await cdp('Runtime.enable');await cdp('Network.enable');
 await cdp('Page.navigate',{url:base+'/'});
 await until(()=>js(`document.querySelector('a[href="/account/"]')`),'landing member entry visible');
 if(!await js(`document.querySelector('a[href="/admin"]') && document.body.textContent.includes('does not enroll a device or grant membership') && document.body.textContent.includes('not live connectivity')`))throw new Error('landing mixed member/operator scope');
 await shot('account-landing');
 await js(`document.querySelector('a[href="/account/"]').click()`);
 await until(()=>js(`document.querySelector('#login') && !document.querySelector('#login').hidden`),'member sign-in visible');
 await shot('account-signed-out');
 await js(`document.querySelector('#network').value=${JSON.stringify(network)};document.querySelector('#loginForm').requestSubmit()`);
 await until(()=>js(`document.querySelector('#devices') && !document.querySelector('#devices').hidden && document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'signed-provider member device render');
 if(!await js(`document.querySelector('#deviceRows').rows.length===2 && document.querySelector('#deviceRows').textContent.includes('personal-phone') && !/same-email-outsider|other-network-device|other-issuer-device|expired-owned-device/.test(document.querySelector('#deviceRows').textContent)`))throw new Error('multi-device ownership scope leaked or omitted devices');
 const actual=await js(`(async()=>{const r=await fetch('/account/api/devices?network_id=other&subject=different-user');return {status:r.status,body:await r.json()};})()`);
 if(actual.status!==200||actual.body.devices.length!==2||actual.body.devices.some(d=>Object.keys(d).some(k=>!['ID','Name','IPv4'].includes(k))))throw new Error('account response scope or summary fields wrong');
 if(!await js(`document.body.textContent.includes('not a live online-status check')`))throw new Error('device list implies online proof');
 await shot('account-signed-in');
 // Rendering boundary: untrusted names must be text, never executable HTML.
 await js(`(async()=>{window.originalFetch=fetch;window.injected=false;window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response(JSON.stringify({devices:[{ID:'fixture-render-only',Name:'<img src=x onerror="window.injected=true">',IPv4:'100.64.0.9'}]}),{status:200,headers:{'Content-Type':'application/json'}})):originalFetch(input,options);await load();})()`);
 if(!await js(`!window.injected && !document.querySelector('#deviceRows img') && document.querySelector('#deviceRows').textContent.includes('<img')`))throw new Error('device name parsed as HTML');
 // Reject the entire response before rendering if any row has wrong shape.
 for(const body of ['null','{}',JSON.stringify({devices:[null]}),JSON.stringify({devices:[{ID:'partial',Name:'would-be-partial',IPv4:'100.64.0.8'},{}]}),JSON.stringify({devices:[{ID:7,Name:'bad',IPv4:'100.64.0.8'}]}),JSON.stringify({devices:[{ID:'bad',Name:{private:'details'},IPv4:'100.64.0.8'}]})]){
  await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response(${JSON.stringify(body)},{status:200,headers:{'Content-Type':'application/json'}})):originalFetch(input,options);await load();})()`);
  if(!await js(`!document.querySelector('#deviceRows').textContent && document.querySelector('#devices').hidden && !document.querySelector('#login').hidden && document.querySelector('#message').textContent==='Could not read your devices. Try again.'`))throw new Error('malformed member response rendered partial private rows or empty-success state');
 }
 // The latest refresh wins when two reads finish in reverse order.
 await js(`(async()=>{window.deviceReleases=[];window.fetch=(input,options)=>input==='/account/api/devices'?new Promise(resolve=>deviceReleases.push(name=>resolve(new Response(JSON.stringify({devices:[{ID:'fixture',Name:name,IPv4:'100.64.0.8'}]}),{status:200,headers:{'Content-Type':'application/json'}})))):originalFetch(input,options);window.olderLoad=load();window.newerLoad=load();deviceReleases[1]('latest-device');await newerLoad;deviceReleases[0]('stale-device');await olderLoad;})()`);
 if(!await js(`document.querySelector('#deviceRows').rows.length===1 && document.querySelector('#deviceRows').textContent.includes('latest-device') && !document.querySelector('#deviceRows').textContent.includes('stale-device')`))throw new Error('older member refresh replaced latest result');
 await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response(JSON.stringify({devices:[]}),{status:200,headers:{'Content-Type':'application/json'}})):originalFetch(input,options);await load();})()`);
 if(!await js(`!document.querySelector('#empty').hidden && !document.querySelector('#deviceRows').textContent`))throw new Error('empty member state incorrect');
 await shot('account-empty');
 await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response('{}',{status:401})):originalFetch(input,options);await load();})()`);
 if(!await js(`document.querySelector('#devices').hidden && !document.querySelector('#deviceRows').textContent && !document.querySelector('#login').hidden`))throw new Error('denied session left private UI');
 await js(`window.fetch=originalFetch;load()`);
 await until(()=>js(`document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'real device restored after display fixtures');
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 if(!await js(`document.documentElement.scrollWidth<=390`))throw new Error('member page overflows narrow viewport');
 await shot('account-mobile');
 await cdp('Emulation.clearDeviceMetricsOverride');
 // A failed logout must not say signed out or restore the previous rows.
 await js(`window.fetch=(input,options)=>input==='/account/api/logout'?Promise.resolve(new Response('{}',{status:503})):originalFetch(input,options);document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent.includes('Could not confirm sign-out')`),'failed sign-out accurately reported');
 if(!await js(`!document.querySelector('#deviceRows').textContent && document.querySelector('#login').hidden`))throw new Error('failed sign-out restored private rows or pretended signed out');
 await js(`window.fetch=originalFetch`);

 const cookie=(await cdp('Network.getCookies',{urls:[base+'/account/']})).cookies.find(c=>c.name==='mnet_account');if(!cookie)throw new Error('account cookie absent');
 // A valid response already in flight cannot repaint private devices after logout.
 await js(`(async()=>{window.savedFetch=fetch;window.held=await(await savedFetch('/account/api/devices')).text();window.releaseHeld=null;window.fetch=(input,options)=>input==='/account/api/devices'?new Promise(resolve=>{window.releaseHeld=()=>resolve(new Response(held,{status:200,headers:{'Content-Type':'application/json'}}));}):savedFetch(input,options);window.pendingLoad=load();})()`);
 await js(`document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent==='Signed out.'`),'account sign-out confirmed');
 await js(`(async()=>{releaseHeld();await pendingLoad;window.fetch=savedFetch;})()`);
 if(!await js(`document.querySelector('#devices').hidden && !document.querySelector('#login').hidden && !document.querySelector('#deviceRows').textContent && !document.querySelector('#network').value`))throw new Error('private device data survived sign-out or late refresh');
 const replay=await fetch(base+'/account/api/devices',{headers:{Cookie:'mnet_account='+cookie.value}});if(replay.status!==401)throw new Error('account session replay after logout accepted');
 await shot('account-after-logout');
 // A logout may commit while only its response is lost. Keep the UI honest,
 // clear private rows, deny old-token replay and let an idempotent retry finish.
 await js(`document.querySelector('#network').value=${JSON.stringify(network)};document.querySelector('#loginForm').requestSubmit()`);
 await until(()=>js(`document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'second real login before lost logout response');
 const lostCookie=(await cdp('Network.getCookies',{urls:[base+'/account/']})).cookies.find(c=>c.name==='mnet_account');if(!lostCookie)throw new Error('second account cookie absent');
 await js(`window.logoutFetch=window.fetch;window.fetch=async(input,options)=>{if(input==='/account/api/logout'){const response=await logoutFetch(input,options);if(!response.ok)throw new Error('fixture logout failed');throw new Error('fixture lost committed logout response');}return logoutFetch(input,options);};document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent.includes('Could not confirm sign-out')`),'lost committed logout kept uncertain');
 if(!await js(`!document.querySelector('#deviceRows').textContent && document.querySelector('#login').hidden`))throw new Error('lost logout response restored private rows or claimed signed out');
 const lostReplay=await fetch(base+'/account/api/devices',{headers:{Cookie:'mnet_account='+lostCookie.value}});if(lostReplay.status!==401)throw new Error('committed logout token remained usable');
 await js(`window.fetch=logoutFetch;document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent==='Signed out.' && document.querySelector('#devices').hidden`),'idempotent logout retry confirmed');
 await cdp('Page.navigate',{url:base+'/account/oidc/callback?state=private-fixture-state&code=private-fixture-code&error=private-provider-error'});
 await until(()=>js(`document.body.textContent.includes('Sign-in was not completed')`),'safe sign-in recovery page');
 if(!await js(`!document.body.textContent.includes('private-fixture') && !document.body.textContent.includes('private-provider-error') && document.querySelector('a[href="/account/"]')`))throw new Error('callback recovery echoed private data or lost return path');
 await shot('account-login-failed');
 await js(`document.querySelector('a[href="/account/"]').click()`);
 await until(()=>js(`document.querySelector('#login') && !document.querySelector('#login').hidden`),'return from failed sign-in');

 console.log('PASS real Chrome account: signed-provider login, explicit member two-device scoped render, HTML-as-text, empty/denied states, narrow layout, failed logout, private UI cleanup, delayed-refresh suppression and logout replay denial');
}finally{ws?.close();chrome?.kill();await delay(150);rmSync(dir,{recursive:true,force:true});}
