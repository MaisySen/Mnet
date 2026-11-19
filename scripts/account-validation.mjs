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
async function shot(name,viewport=false){if(!process.env.MNET_ACCOUNT_SHOTS)return;const r=await cdp('Page.captureScreenshot',{format:'png',captureBeyondViewport:!viewport});writeFileSync(join(process.env.MNET_ACCOUNT_SHOTS,name+'.png'),Buffer.from(r.data,'base64'));}
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
 // The disposable operator starts with no grant. Create it through the real
 // reviewed form, then cross into the separate signed-provider member flow.
 const fixturePassword=process.env.MNET_ACCOUNT_FIXTURE_PASSWORD,fixtureIssuer=process.env.MNET_ACCOUNT_FIXTURE_ISSUER;
 if(!fixturePassword||!fixtureIssuer)throw new Error('operator fixture missing');
 await cdp('Page.navigate',{url:base+'/admin'});
 await until(()=>js(`document.querySelector('#loginBtn') && getComputedStyle(document.querySelector('#login')).display!=='none'`),'operator setup login');
 await js(`document.querySelector('#pw').value=${JSON.stringify(fixturePassword)};document.querySelector('#loginBtn').click()`);
 await until(()=>js(`state && document.querySelector('#memberNet').options.length>=2`),'operator setup networks');
 await js(`document.querySelector('#memberNet').value=${JSON.stringify(network)};document.querySelector('#memberNet').dispatchEvent(new Event('change'));document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent==='No explicit grants for this network.'`),'member initially has no explicit grant');
 await js(`window.confirm=()=>true;document.querySelector('#grantDetails').open=true;document.querySelector('#grantIssuer').value=${JSON.stringify(fixtureIssuer)};document.querySelector('#grantSubject').value='user-1';document.querySelector('#grantVerified').checked=true;document.querySelector('#memberGrant').requestSubmit()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('user-1')`),'operator reviewed grant reached real store');
 await cdp('Page.navigate',{url:base+'/'});
 await until(()=>js(`document.querySelector('a[href="/account/"]')`),'landing member entry visible');
 if(!await js(`document.querySelector('a[href="/admin"]') && document.body.textContent.includes('does not enroll a device or grant membership') && document.body.textContent.includes('not live connectivity')`))throw new Error('landing mixed member/operator scope');
 await shot('account-landing');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('landing overflows compact viewport');
 await shot('landing-320');
 await js(`document.documentElement.style.fontSize='200%'`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('landing overflows200percent text');
 await shot('landing-320-large');
 await cdp('Emulation.clearDeviceMetricsOverride');
 await js(`document.documentElement.style.fontSize=''`);
 await js(`document.querySelector('a[href="/account/"]').click()`);
 await until(()=>js(`document.querySelector('#login') && !document.querySelector('#login').hidden`),'member sign-in visible');
 if(!await js(`document.activeElement.id==='network'`))throw new Error('member login keyboard focus missing');
 await js(`document.querySelector('#network').value='   ';document.querySelector('#loginForm').requestSubmit()`);
 if(!await js(`document.activeElement.id==='network' && document.querySelector('#network').getAttribute('aria-invalid')==='true'`))throw new Error('blank network did not recover field');
 await js(`window.signInFetch=fetch;window.signInCalls=0;window.fetch=(input,options)=>input==='/account/api/login'?(window.signInCalls++,new Promise(resolve=>window.releaseSignIn=()=>resolve(new Response('{}',{status:503})))):signInFetch(input,options);document.querySelector('#network').value='fixture-network';document.querySelector('#loginForm').requestSubmit();document.querySelector('#loginForm').requestSubmit();`);
 if(!await js(`signInCalls===1 && document.querySelector('#signIn').disabled`))throw new Error('member sign-in repeated pending request');
 await js(`releaseSignIn()`);
 await until(()=>js(`!document.querySelector('#signIn').disabled && document.querySelector('#message').textContent.includes('Could not start')`),'member sign-in retry available');
 if(!await js(`document.activeElement.id==='network' && document.querySelector('#network').value==='fixture-network' && document.querySelector('#network').getAttribute('aria-invalid')!=='true'`))throw new Error('member sign-in failure lost field or marked uncertain ID invalid');
 await js(`window.fetch=signInFetch`);
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%';document.querySelector('#network').scrollIntoView({block:'center'})`);
 await cdp('Input.dispatchKeyEvent',{type:'keyDown',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});
 await cdp('Input.dispatchKeyEvent',{type:'keyUp',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});
 if(!await js(`document.activeElement.id==='signIn' && document.documentElement.scrollWidth<=320`))throw new Error('member keyboard retry unreachable at enlarged text');
 await js(`document.querySelector('#loginFeedback').scrollIntoView({block:'start'})`);
 if(!await js(`document.querySelector('#loginFeedback').textContent.includes('Could not start') && document.querySelector('#signIn').getBoundingClientRect().bottom<=innerHeight`))throw new Error('member inline failure and retry cannot share viewport');
 await shot('account-login-retry-320',true);
 await cdp('Emulation.clearDeviceMetricsOverride');
 await js(`document.documentElement.style.fontSize='100%'`);
 await shot('account-signed-out');
 await js(`document.querySelector('#network').value=${JSON.stringify(network)};document.querySelector('#loginForm').requestSubmit()`);
 await until(()=>js(`document.querySelector('#devices') && !document.querySelector('#devices').hidden && document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'signed-provider member device render');
 if(!await js(`document.querySelector('#deviceRows').rows.length===2 && document.querySelector('#deviceRows').textContent.includes('personal-phone') && !/same-email-outsider|other-network-device|other-issuer-device|expired-owned-device/.test(document.querySelector('#deviceRows').textContent)`))throw new Error('multi-device ownership scope leaked or omitted devices');
 const actual=await js(`(async()=>{const r=await fetch('/account/api/devices?network_id=other&subject=different-user');return {status:r.status,body:await r.json()};})()`);
 if(actual.status!==200||actual.body.network_id!==network||actual.body.devices.length!==2||actual.body.devices.some(d=>Object.keys(d).some(k=>!['ID','Name','IPv4'].includes(k))))throw new Error('account response scope or summary fields wrong');
 if(!await js(`document.body.textContent.includes('not a live online-status check')`))throw new Error('device list implies online proof');
 if(!await js(`document.querySelector('#currentNetwork').textContent===${JSON.stringify(network)}`))throw new Error('member view omitted authenticated network context');
 if(!await js(`document.activeElement.id==='devicesHeading'`))throw new Error('member entered view focus missing');
 await js(`(async()=>{document.querySelector('#refresh').focus();await load();})()`);
 if(!await js(`document.activeElement.id==='refresh'`))throw new Error('member refresh stole focus');
 await shot('account-signed-in');
 // Rendering boundary: untrusted names must be text, never executable HTML.
 await js(`(async()=>{window.originalFetch=fetch;window.injected=false;window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response(JSON.stringify({network_id:${JSON.stringify(network)},devices:[{ID:'fixture-render-only',Name:'<img src=x onerror="window.injected=true">',IPv4:'100.64.0.9'}]}),{status:200,headers:{'Content-Type':'application/json'}})):originalFetch(input,options);await load();})()`);
 if(!await js(`!window.injected && !document.querySelector('#deviceRows img') && document.querySelector('#deviceRows').textContent.includes('<img')`))throw new Error('device name parsed as HTML');
 // Reject the entire response before rendering if any row has wrong shape.
 for(const body of ['null','{}',JSON.stringify({devices:[]}),JSON.stringify({network_id:null,devices:[]}),JSON.stringify({network_id:7,devices:[]}),JSON.stringify({network_id:'',devices:[]}),JSON.stringify({network_id:network,devices:[null]}),JSON.stringify({network_id:network,devices:[{ID:'partial',Name:'would-be-partial',IPv4:'100.64.0.8'},{}]}),JSON.stringify({network_id:network,devices:[{ID:7,Name:'bad',IPv4:'100.64.0.8'}]}),JSON.stringify({network_id:network,devices:[{ID:'bad',Name:{private:'details'},IPv4:'100.64.0.8'}]})]){
  await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response(${JSON.stringify(body)},{status:200,headers:{'Content-Type':'application/json'}})):originalFetch(input,options);await load();})()`);
  if(!await js(`!document.querySelector('#deviceRows').textContent && !document.querySelector('#currentNetwork').textContent && document.querySelector('#devices').hidden && document.querySelector('#login').hidden && !document.querySelector('#sessionRecovery').hidden && document.querySelector('#message').textContent==='Could not read your devices. Try again.'`))throw new Error('malformed member response rendered partial private rows or empty-success state');
 }
 // Service failure must not claim signed-out state or require a new identity-provider login.
 for(const mode of ['service','connection']){
  await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?${mode==='service'?"Promise.resolve(new Response('{}',{status:503}))":"Promise.reject(new TypeError('fixture offline'))"}:originalFetch(input,options);await load();})()`);
  if(!await js(`document.querySelector('#login').hidden && !document.querySelector('#sessionRecovery').hidden && document.querySelector('#devices').hidden && !document.querySelector('#deviceRows').textContent && document.activeElement.id==='sessionRecoveryHeading'`))throw new Error('read unavailable incorrectly signed out or left stale private data');
 }
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await js(`document.querySelector('#sessionRecovery').scrollIntoView({block:'start'})`);
 await shot('account-device-read-retry-390',true);
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%';document.querySelector('#retryDevices').scrollIntoView({block:'center'});document.querySelector('#retryDevices').focus()`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('read retry overflows enlarged narrow viewport');
 await shot('account-device-read-retry-320',true);
 await js(`document.documentElement.style.fontSize='';window.fetch=(input,options)=>input==='/account/api/logout'?Promise.reject(new TypeError('fixture logout offline')):originalFetch(input,options);document.querySelector('#recoveryLogout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent==='Could not confirm sign-out. Retry before leaving this browser.' && document.activeElement.id==='retrySignout'`),'unavailable-read sign-out recovery');
 await js(`window.fetch=originalFetch;document.querySelector('#inspectSession').click()`);
 await until(()=>js(`!document.querySelector('#devices').hidden && document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'same session survives failed logout');
 // Test the visible retry itself on a second failed read.
 await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response('{}',{status:503})):originalFetch(input,options);await load();window.fetch=originalFetch;document.querySelector('#retryDevices').click();})()`);

 await until(()=>js(`!document.querySelector('#devices').hidden && document.querySelector('#sessionRecovery').hidden && document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'same-session direct device retry');
 await cdp('Emulation.setDeviceMetricsOverride',{width:1280,height:1000,deviceScaleFactor:1,mobile:false});
 // The latest refresh wins when two reads finish in reverse order.
 await js(`(async()=>{window.deviceReleases=[];window.fetch=(input,options)=>input==='/account/api/devices'?new Promise(resolve=>deviceReleases.push(name=>resolve(new Response(JSON.stringify({network_id:${JSON.stringify(network)},devices:[{ID:'fixture',Name:name,IPv4:'100.64.0.8'}]}),{status:200,headers:{'Content-Type':'application/json'}})))):originalFetch(input,options);window.olderLoad=load();window.newerLoad=load();deviceReleases[1]('latest-device');await newerLoad;deviceReleases[0]('stale-device');await olderLoad;})()`);
 if(!await js(`document.querySelector('#deviceRows').rows.length===1 && document.querySelector('#deviceRows').textContent.includes('latest-device') && !document.querySelector('#deviceRows').textContent.includes('stale-device')`))throw new Error('older member refresh replaced latest result');
 await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response(JSON.stringify({network_id:${JSON.stringify(network)},devices:[]}),{status:200,headers:{'Content-Type':'application/json'}})):originalFetch(input,options);await load();})()`);
 if(!await js(`!document.querySelector('#empty').hidden && !document.querySelector('#deviceRows').textContent`))throw new Error('empty member state incorrect');
 await shot('account-empty');
 await js(`(async()=>{window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response('{}',{status:401})):originalFetch(input,options);await load();})()`);
 if(!await js(`document.querySelector('#devices').hidden && !document.querySelector('#deviceRows').textContent && !document.querySelector('#currentNetwork').textContent && !document.querySelector('#login').hidden`))throw new Error('denied session left private UI');
 await js(`window.fetch=originalFetch;load()`);
 await until(()=>js(`document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'real device restored after display fixtures');
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 if(!await js(`document.documentElement.scrollWidth<=390`))throw new Error('member page overflows narrow viewport');
 await shot('account-mobile');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%'`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('member page overflows compact enlarged text');
 await shot('account-320-large');
 if(!await js(`document.querySelector('.device-table').scrollWidth<=document.querySelector('.device-table').clientWidth && Array.from(document.querySelectorAll('#deviceRows .device-field-label')).every(label=>getComputedStyle(label).display==='block')`))throw new Error('member mobile records lose labels or overflow');
 if(process.env.MNET_ACCOUNT_SHOTS){const clip=await js(`(()=>{const r=document.querySelector('.device-table').getBoundingClientRect();return {x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1};})()`);const image=await cdp('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip});writeFileSync(join(process.env.MNET_ACCOUNT_SHOTS,'account-records-320-large.png'),Buffer.from(image.data,'base64'));}

 await js(`document.querySelector('#logout').scrollIntoView({block:'center'})`);
 if(!await js(`document.querySelector('#logout').getBoundingClientRect().right<=innerWidth && document.querySelector('#logout').getBoundingClientRect().height>=48`))throw new Error('member logout not usable at enlarged text');
 await shot('account-320-large-bottom',true);
 await js(`document.documentElement.style.fontSize='';window.scrollTo(0,0)`);
 await cdp('Emulation.clearDeviceMetricsOverride');
 await until(()=>js(`innerWidth>600 && getComputedStyle(document.querySelector('#deviceRows tr')).display==='table-row' && getComputedStyle(document.querySelector('.device-field-label')).display==='none'`),'member desktop table restored');
 // A late sign-in response must not navigate away after an intervening
 // authenticated view has been signed out. Hold a real-form request while
 // a session recheck supplies the already-authenticated browser state.
 for(const mode of ['headers','json','error']) {
 await js(`window.lateLoginFetch=fetch;window.fetch=(input,options)=>input==='/account/api/devices'?Promise.resolve(new Response('{}',{status:401})):lateLoginFetch(input,options);load()`);
 await until(()=>js(`!document.querySelector('#login').hidden`),'late-login fixture shows sign-in');
 const held=mode==='json'?`Promise.resolve({ok:true,json:()=>new Promise(resolve=>window.finishLateLogin=()=>resolve({authorization_url:location.origin+'/account/?retired-login-response=1'}))})`:mode==='error'?`new Promise((resolve,reject)=>window.finishLateLogin=()=>reject(new TypeError('fixture retired request')))`:`new Promise(resolve=>window.finishLateLogin=()=>resolve(new Response(JSON.stringify({authorization_url:location.origin+'/account/?retired-login-response=1'}),{status:200,headers:{'Content-Type':'application/json'}})))`;
 await js(`window.finishLateLogin=null;window.fetch=(input,options)=>input==='/account/api/login'?${held}:lateLoginFetch(input,options);document.querySelector('#network').value=${JSON.stringify(network)};document.querySelector('#loginForm').requestSubmit()`);
 await until(()=>js(`typeof window.finishLateLogin==='function'`),'held sign-in '+mode);
 await js(`load()`);
 await until(()=>js(`!document.querySelector('#devices').hidden`),'intervening authenticated session recheck');
 await js(`document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent==='Signed out.'`),'intervening session sign-out');
 await js(`window.fetch=(input,options)=>input==='/account/api/login'?new Promise(resolve=>window.finishNewerLogin=()=>resolve(new Response('{}',{status:503}))):lateLoginFetch(input,options);document.querySelector('#network').value='newer-attempt';document.querySelector('#loginForm').requestSubmit();window.lateLoginMarker=true;finishLateLogin()`);
 await delay(100);
 if(!await js(`window.lateLoginMarker===true && !location.search && document.querySelector('#message').textContent==='Opening your identity provider...' && document.querySelector('#signIn').disabled && document.querySelector('#network').value==='newer-attempt'`))throw new Error('retired sign-in response navigated or changed newer attempt');
 await js(`finishNewerLogin()`);
 await until(()=>js(`!document.querySelector('#signIn').disabled && document.querySelector('#message').textContent.includes('Could not start')`),'newer sign-in owns retry after stale completion');
 await js(`new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))`);
 await delay(250);
 await shot('account-stale-signin-retry-'+mode);
 await js(`window.fetch=lateLoginFetch;document.querySelector('#network').value=${JSON.stringify(network)};document.querySelector('#loginForm').requestSubmit()`);
 await until(()=>js(`document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'fresh login after retired response');
 await js(`window.originalFetch=fetch`);

 }

 // A failed logout must not say signed out or restore the previous rows.
 await js(`window.fetch=(input,options)=>input==='/account/api/logout'?Promise.resolve(new Response('{}',{status:503})):originalFetch(input,options);document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent.includes('Could not confirm sign-out')`),'failed sign-out accurately reported');
 if(!await js(`!document.querySelector('#deviceRows').textContent && !document.querySelector('#currentNetwork').textContent && document.querySelector('#login').hidden`))throw new Error('failed sign-out restored private rows or pretended signed out');
 if(!await js(`document.activeElement.id==='retrySignout'`))throw new Error('failed logout retry lost focus');
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await js(`document.querySelector('#message').scrollIntoView({block:'start'})`);
 await shot('account-signout-unconfirmed-390',true);
 if(!await js(`document.querySelector('#devices').hidden && !document.querySelector('#logoutRecovery').hidden && document.activeElement.id==='retrySignout'`))throw new Error('unconfirmed sign-out shows an empty account rather than recovery');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%';document.querySelector('#retrySignout').scrollIntoView({block:'center'})`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('unconfirmed sign-out recovery overflows enlarged viewport');
 await shot('account-signout-unconfirmed-320',true);
 await js(`document.documentElement.style.fontSize=''`);

 await cdp('Emulation.clearDeviceMetricsOverride');

 await js(`window.fetch=originalFetch`);

 const cookie=(await cdp('Network.getCookies',{urls:[base+'/account/']})).cookies.find(c=>c.name==='mnet_account');if(!cookie)throw new Error('account cookie absent');
 // A valid response already in flight cannot repaint private devices after logout.
 await js(`(async()=>{window.savedFetch=fetch;window.held=await(await savedFetch('/account/api/devices')).text();window.releaseHeld=null;window.fetch=(input,options)=>input==='/account/api/devices'?new Promise(resolve=>{window.releaseHeld=()=>resolve(new Response(held,{status:200,headers:{'Content-Type':'application/json'}}));}):savedFetch(input,options);window.pendingLoad=load();})()`);
 await js(`document.querySelector('#retrySignout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent==='Signed out.'`),'account sign-out confirmed');
 await js(`(async()=>{releaseHeld();await pendingLoad;window.fetch=savedFetch;})()`);
 if(!await js(`document.querySelector('#devices').hidden && !document.querySelector('#login').hidden && !document.querySelector('#deviceRows').textContent && !document.querySelector('#network').value && !document.querySelector('#currentNetwork').textContent`))throw new Error('private device data survived sign-out or late refresh');
 const replay=await fetch(base+'/account/api/devices',{headers:{Cookie:'mnet_account='+cookie.value}});if(replay.status!==401)throw new Error('account session replay after logout accepted');
 if(!await js(`document.activeElement.id==='network'`))throw new Error('member sign-out focus missing');
 await shot('account-after-logout');
 await cdp('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 if(!await js(`matchMedia('(prefers-color-scheme: dark)').matches && getComputedStyle(document.documentElement).colorScheme==='light' && getComputedStyle(document.body).color==='rgb(29, 29, 31)' && getComputedStyle(document.querySelector('#network')).backgroundColor==='rgb(255, 255, 255)'`))throw new Error('supported light palette changed under dark system preference');
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await js(`document.querySelector('#network').scrollIntoView({block:'center'})`);
 await shot('member-system-dark-light-ui',true);
 await cdp('Emulation.setEmulatedMedia',{features:[{name:'forced-colors',value:'active'}]});
 if(!await js(`matchMedia('(forced-colors: active)').matches`))throw new Error('forced colors emulation not active');
 if(!await js(`getComputedStyle(document.querySelector('#signIn')).borderTopStyle==='solid' && parseFloat(getComputedStyle(document.querySelector('#signIn')).borderTopWidth)>=1`))throw new Error('forced-colors action boundary missing');
 await shot('member-forced-colors',true);

 await cdp('Emulation.setEmulatedMedia',{features:[]});
 await cdp('Emulation.clearDeviceMetricsOverride');

 // A logout may commit while only its response is lost. Keep the UI honest,
 // clear private rows, deny old-token replay and let an idempotent retry finish.
 await js(`document.querySelector('#network').value=${JSON.stringify(network)};document.querySelector('#loginForm').requestSubmit()`);
 await until(()=>js(`document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'second real login before lost logout response');
 const lostCookie=(await cdp('Network.getCookies',{urls:[base+'/account/']})).cookies.find(c=>c.name==='mnet_account');if(!lostCookie)throw new Error('second account cookie absent');
 await js(`window.logoutFetch=window.fetch;window.fetch=async(input,options)=>{if(input==='/account/api/logout'){const response=await logoutFetch(input,options);if(!response.ok)throw new Error('fixture logout failed');throw new Error('fixture lost committed logout response');}return logoutFetch(input,options);};document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent.includes('Could not confirm sign-out')`),'lost committed logout kept uncertain');
 if(!await js(`!document.querySelector('#deviceRows').textContent && !document.querySelector('#currentNetwork').textContent && document.querySelector('#login').hidden`))throw new Error('lost logout response restored private rows or claimed signed out');
 const lostReplay=await fetch(base+'/account/api/devices',{headers:{Cookie:'mnet_account='+lostCookie.value}});if(lostReplay.status!==401)throw new Error('committed logout token remained usable');
 await js(`window.fetch=logoutFetch;document.querySelector('#retrySignout').click()`);
 await until(()=>js(`document.querySelector('#message').textContent==='Signed out.' && document.querySelector('#devices').hidden`),'idempotent logout retry confirmed');
 // Existing operator session can remove the grant; an already active member
 // session must immediately lose its query without a new login round trip.
 await js(`document.querySelector('#network').value=${JSON.stringify(network)};document.querySelector('#loginForm').requestSubmit()`);
 await until(()=>js(`document.querySelector('#deviceRows').textContent.includes('personal-laptop')`),'third real member login before operator removal');
 await cdp('Page.navigate',{url:base+'/admin'});
 await until(()=>js(`state && document.querySelector('#memberNet').options.length>=2`),'existing operator session restored');
 await js(`document.querySelector('#memberNet').value=${JSON.stringify(network)};document.querySelector('#memberNet').dispatchEvent(new Event('change'));document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#members tbody button')`),'granted principal visible for removal');
 await js(`window.confirm=()=>true;document.querySelector('#members tbody button').click()`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent==='No explicit grants for this network.'`),'operator removed actual member grant');
 const revoked=await js(`(async()=>{const response=await fetch('/account/api/devices');return response.status;})()`);if(revoked!==401)throw new Error('active member survived operator UI removal');
 await cdp('Page.navigate',{url:base+'/account/'});
 await until(()=>js(`document.querySelector('#login') && !document.querySelector('#login').hidden && document.querySelector('#devices').hidden && !document.querySelector('#currentNetwork').textContent && !document.querySelector('#deviceRows').textContent`),'revoked account returned to cleared sign-in');
 await cdp('Page.navigate',{url:base+'/account/oidc/callback?state=private-fixture-state&code=private-fixture-code&error=private-provider-error'});
 await until(()=>js(`document.body.textContent.includes('Sign-in was not completed')`),'safe sign-in recovery page');
 if(!await js(`!document.body.textContent.includes('private-fixture') && !document.body.textContent.includes('private-provider-error') && document.querySelector('a[href="/account/"]')`))throw new Error('callback recovery echoed private data or lost return path');
 await shot('account-login-failed');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%'`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('recovery page overflows enlarged text');
 await shot('account-recovery-320-large');
 await cdp('Emulation.clearDeviceMetricsOverride');
 await js(`document.querySelector('a[href="/account/"]').click()`);
 await until(()=>js(`document.querySelector('#login') && !document.querySelector('#login').hidden`),'return from failed sign-in');


 // Actual local provider redirect/code exchange and persisted node approval.
 const deviceApprovalURL=process.env.MNET_DEVICE_APPROVAL_URL;
 if(!deviceApprovalURL)throw new Error('device approval fixture missing');
 await cdp('Page.navigate',{url:deviceApprovalURL});
 await until(()=>js(`document.querySelector('h1')?.textContent==='Device approved'`),'verified device approval success');
 if(!await js(`document.body.textContent.includes('does not confirm a live VPN connection') && !document.body.textContent.includes('This device is now on the mesh') && document.querySelector('b').textContent==='dev@fintex.au'`))throw new Error('approval success misstates connectivity or lost verified label');
 await shot('device-approved');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%'`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('approval success overflows enlarged text');
 await shot('device-approved-320-large');
 await js(`document.querySelector('.hint').scrollIntoView({block:'center'})`);
 await shot('device-approved-320-status',true);
 if(await js(`(async()=>{const r=await fetch('/account/api/devices');return r.status})()`)!==401)throw new Error('device approval created member access');
 console.log('PASS real Chrome account: labeled enlarged member records with desktop restoration, dedicated unconfirmed-signout retry/session check, whitespace/pending login guard, failed sign-in keyboard retry, focus transitions and refresh retention, signed-provider login, explicit member two-device scoped render, HTML-as-text, empty/denied states, narrow layout, unavailable/malformed read recovery without new sign-in, direct same-session read retry, failed recovery logout, private UI cleanup, delayed-refresh suppression, logout replay denial and verified device-approval success without member access');
}finally{ws?.close();chrome?.kill();await delay(150);rmSync(dir,{recursive:true,force:true});}
