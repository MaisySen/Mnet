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
async function until(fn,description){const end=Date.now()+15000;let last;while(Date.now()<end){try{const value=await fn();if(value)return value;}catch(e){last=e;}await delay(50);}throw new Error('Timed out: '+description+(last?' '+last.message:''));}
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
 chrome=spawn(executable,['--headless','--no-sandbox','--disable-gpu','--remote-debugging-port='+debugPort,'--user-data-dir='+join(dir,'chrome'),'--window-size=1280,1000','about:blank'],{stdio:'ignore'});
 const target=await until(async()=>{const tabs=await(await fetch('http://127.0.0.1:'+debugPort+'/json')).json();return tabs.find(t=>t.type==='page');},'Chrome target');
 ws=new WebSocket(target.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j;});ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id&&pending.has(m.id)){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(new Error(m.error.message)):p.resolve(m.result);}};
 await cdp('Page.enable');await cdp('Runtime.enable');await cdp('Network.enable');
 await cdp('Page.navigate',{url:base+'/admin'});
 await until(()=>js(`document.querySelector('#login') && getComputedStyle(document.querySelector('#login')).display!=='none'`),'initial login visible');await shot('dashboard-login');
 await js(`document.querySelector('#pw').value='local-browser-fixture';document.querySelector('#loginBtn').click()`);
 await until(()=>js(`getComputedStyle(document.querySelector('#app')).display!=='none' && getComputedStyle(document.querySelector('#login')).display==='none'`),'signed-in app only');
 await until(()=>js(`document.querySelector('#nets tbody').textContent.includes('fixture')`),'real network rendered');if(!await js(`document.querySelector('#app').textContent.includes('Exit-node selection and internet routing are unsupported') && !document.querySelector('#app').textContent.includes('0.0.0.0/0 for exit-node internet access')`))throw new Error('ACL help implies unsupported exit routing');await shot('dashboard-signed-in');
 // Exercise actual key creation without recording or printing its credential.
 await js(`window.prompt=()=>null;document.querySelector('#keyNote').value='cleanup-fixture';document.querySelector('#keyTTL').value='1';document.querySelector('#newKey').requestSubmit()`);
 await until(()=>js(`document.querySelector('#keys tbody').textContent.includes('cleanup-fixture') && /[0-9]+[ms] left/.test(document.querySelector('#keys tbody').textContent)`),'populated unexpired key row');
 if(process.env.MNET_DASHBOARD_SHOTS){
  await js(`document.querySelectorAll('#keys .key code').forEach(cell=>cell.textContent='[fixture credential hidden]')`);
  await shot('dashboard-expiry');
 }

 if(!await js(`(()=>{const now=Date.now();return expires('',now)==='never' && expires('invalid',now)==='unknown' && expires(new Date(now).toISOString(),now)==='expired' && expires(new Date(now-1000).toISOString(),now)==='expired' && expires(new Date(now+30000).toISOString(),now)==='30s left' && expires(new Date(now+120000).toISOString(),now)==='2m left' && expires(new Date(now+7200000).toISOString(),now)==='2h left' && expires(new Date(now+172800000).toISOString(),now)==='2d left';})()`))throw new Error('expiry formatting boundary failed');
 const cookies=(await cdp('Network.getCookies',{urls:[base]})).cookies;const token=cookies.find(c=>c.name==='mnet_admin')?.value;if(!token)throw new Error('session cookie missing');
 // Capture a valid old state response and hold a refresh after logout starts.
 await js(`(async()=>{window.savedFetch=window.fetch;window.heldResponse=await (await savedFetch('/admin/api/state')).text();window.releaseHeld=null;window.fetch=(input,options)=>input==='/admin/api/state'?new Promise(resolve=>{window.releaseHeld=()=>resolve(new Response(heldResponse,{status:200,headers:{'Content-Type':'application/json'}}));}):savedFetch(input,options);window.pendingRefresh=load();})()`);
 await js(`document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#loginMsg').textContent==='Signed out.'`),'logout complete');
 await js(`(async()=>{window.releaseHeld();await window.pendingRefresh;window.fetch=window.savedFetch;})()`);
 if(!await js(`state===null && getComputedStyle(document.querySelector('#app')).display==='none' && getComputedStyle(document.querySelector('#logout')).display==='none' && !document.querySelector('#nets tbody').textContent && !document.querySelector('#keys tbody').textContent && !document.querySelector('#keyNet').textContent`))throw new Error('late refresh restored private dashboard state');
 const replay=await fetch(base+'/admin/api/state',{headers:{Cookie:'mnet_admin='+token}});if(replay.status!==401)throw new Error('logged-out cookie replay accepted');
 const remaining=(await cdp('Network.getCookies',{urls:[base]})).cookies;if(remaining.some(c=>c.name==='mnet_admin'))throw new Error('logout cookie not cleared');
 await shot('dashboard-signed-out');
 console.log('PASS real Chrome dashboard: login visibility, same-origin sign-in, network/key render, expiry boundaries, sign-out, cleared populated private UI, delayed-refresh suppression and logged-out token replay');
}finally{ws?.close();chrome?.kill();control?.kill();await delay(150);rmSync(dir,{recursive:true,force:true});}
