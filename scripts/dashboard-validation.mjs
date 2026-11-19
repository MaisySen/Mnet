// Isolated real-Chrome operator dashboard regression. No user profiles or live
// control service. Node 22+ and Chrome are required. Screenshots stay local.
import {spawn, spawnSync} from 'node:child_process';
import {stopChild} from './fixture-process.mjs';
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
async function shot(name,viewport=false){if(!process.env.MNET_DASHBOARD_SHOTS)return;const r=await cdp('Page.captureScreenshot',{format:'png',captureBeyondViewport:!viewport});writeFileSync(join(process.env.MNET_DASHBOARD_SHOTS,name+'.png'),Buffer.from(r.data,'base64'));}
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
 const initialFault=await cdp('Page.addScriptToEvaluateOnNewDocument',{source:`window.initialStateFetch=fetch;window.fetch=(input,options)=>input==='/admin/api/state'?Promise.resolve(new Response('{}',{status:503})):initialStateFetch(input,options);`});
 await cdp('Page.navigate',{url:base+'/admin'});
 await until(()=>js(`document.querySelector('#stateReadMsg')?.textContent.includes('does not confirm sign-out')`),'first-read service recovery');
 if(!await js(`state===null && !stateFresh && document.querySelector('#dashboardControls').disabled && !document.querySelector('#refresh').classList.contains('hidden') && !document.querySelector('#logout').classList.contains('hidden')`))throw new Error('first-read failure stranded session with no retry/logout');
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await shot('operator-initial-read-recovery',true);
 await cdp('Emulation.clearDeviceMetricsOverride');
 await cdp('Page.removeScriptToEvaluateOnNewDocument',{identifier:initialFault.identifier});
 await js(`window.fetch=initialStateFetch;document.querySelector('#stateReadRetry').click()`);

 await until(()=>js(`document.querySelector('#login') && getComputedStyle(document.querySelector('#login')).display!=='none'`),'initial login visible');await shot('dashboard-login');
 if(!await js(`document.activeElement.id==='pw'`))throw new Error('initial login lost keyboard focus');
 await js(`document.querySelector('#loginBtn').click()`);
 if(!await js(`document.activeElement.id==='pw' && document.querySelector('#pw').getAttribute('aria-invalid')==='true' && document.querySelector('#loginMsg').textContent.includes('Enter')`))throw new Error('empty login lost field recovery');
 await js(`document.querySelector('#pw').value='incorrect-fixture';document.querySelector('#loginBtn').click()`);
 await until(()=>js(`!document.querySelector('#loginBtn').disabled && document.querySelector('#loginMsg').textContent.includes('not accepted')`),'bad password recovery');
 if(!await js(`document.activeElement.id==='pw' && !document.querySelector('#pw').value`))throw new Error('bad password was retained or lost focus');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%'`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('rejected login overflows enlarged text');
 await js(`document.querySelector('#pw').scrollIntoView({block:'center'})`);
 await cdp('Input.dispatchKeyEvent',{type:'keyDown',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});
 await cdp('Input.dispatchKeyEvent',{type:'keyUp',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});
 if(!await js(`document.activeElement.id==='loginBtn'`))throw new Error('keyboard retry button unreachable');
 await shot('dashboard-login-rejected-320',true);
 await cdp('Emulation.clearDeviceMetricsOverride');
 await js(`document.documentElement.style.fontSize='100%'`);
 await shot('dashboard-login-rejected');
 await js(`window.loginFetch=window.fetch;window.loginCalls=0;window.fetch=(input,options)=>input==='/admin/api/login'?(window.loginCalls++,new Promise(resolve=>window.releaseLogin=()=>resolve(new Response('{}',{status:503})))):loginFetch(input,options);document.querySelector('#pw').value='local-browser-fixture';document.querySelector('#loginBtn').click();document.querySelector('#pw').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter'}));`);
 if(!await js(`loginCalls===1 && document.querySelector('#loginBtn').disabled`))throw new Error('login submitted twice while pending');
 await js(`releaseLogin()`);
 await until(()=>js(`!document.querySelector('#loginBtn').disabled && document.querySelector('#loginMsg').textContent.includes('connection')`),'network failure recovery');
 if(!await js(`document.activeElement.id==='pw' && document.querySelector('#pw').getAttribute('aria-invalid')!=='true'`))throw new Error('network failure mislabelled as rejected password');
 await js(`window.fetch=loginFetch;document.querySelector('#pw').dispatchEvent(new Event('input'));document.querySelector('#loginBtn').click()`);
 await until(()=>js(`getComputedStyle(document.querySelector('#app')).display!=='none' && getComputedStyle(document.querySelector('#login')).display==='none'`),'signed-in app only');
 if(!await js(`document.activeElement.id==='overviewHeading'`))throw new Error('signed-in focus remained in hidden login');
 await until(()=>js(`document.querySelector('#nets tbody').textContent.includes('fixture')`),'real network rendered');if(!await js(`document.querySelector('#app').textContent.includes('Exit-node selection and internet routing are unsupported') && !document.querySelector('#app').textContent.includes('0.0.0.0/0 for exit-node internet access')`))throw new Error('ACL help implies unsupported exit routing');await shot('dashboard-signed-in');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('operator page overflows compact viewport');
 await js(`(async()=>{document.querySelector('#keyNote').focus();await load();})()`);
 if(!await js(`document.activeElement.id==='keyNote'`))throw new Error('background refresh stole keyboard focus');
 await shot('dashboard-320');
 await js(`document.querySelector('#members').scrollIntoView();document.querySelector('#members').focus()`);
 await cdp('Input.dispatchKeyEvent',{type:'keyDown',key:'ArrowRight',code:'ArrowRight',windowsVirtualKeyCode:39});
 await cdp('Input.dispatchKeyEvent',{type:'keyUp',key:'ArrowRight',code:'ArrowRight',windowsVirtualKeyCode:39});
 await until(()=>js(`document.querySelector('#members').scrollLeft>0`),'keyboard table scrolling');
 await js(`(()=>{const t=document.querySelector('#members');t.scrollLeft=t.scrollWidth})()`);
 if(!await js(`(()=>{const t=document.querySelector('#members');return t.scrollLeft+t.clientWidth>=t.scrollWidth-2})()`))throw new Error('membership table end unreachable');
 await shot('dashboard-320-membership-table-end',true);

 await js(`document.documentElement.style.fontSize='200%'`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('operator page overflows enlarged text');
 await js(`document.querySelector('#membershipSection').scrollIntoView()`);
 await shot('dashboard-320-large',true);
 await js(`document.querySelector('#policySection').scrollIntoView()`);
 if(!await js(`document.querySelector('#aclSave').getBoundingClientRect().right<=innerWidth && document.querySelector('#aclSave').getBoundingClientRect().height>=44`))throw new Error('operator policy action clipped');
 await shot('dashboard-320-large-policy',true);
 await js(`document.querySelector('#aclSave').scrollIntoView({block:'center'})`);
 if(!await js(`document.querySelector('#aclText').getBoundingClientRect().bottom<=document.querySelector('#aclSave').getBoundingClientRect().top`))throw new Error('policy save does not follow edited rules');
 await shot('dashboard-320-large-policy-action',true);
 await js(`document.documentElement.style.fontSize='';window.scrollTo(0,0)`);
 await cdp('Emulation.clearDeviceMetricsOverride');

 // ACL editor: only disposable fixture networks, never a live policy.
 await until(()=>js(`!document.querySelector('#aclText').disabled && aclTarget`),'initial policy loaded');
 await js(`window.policyFetch=window.fetch;window.policyNet=document.querySelector('#aclNet').value;document.querySelector('#aclText').value='invalid-json';document.querySelector('#aclText').dispatchEvent(new Event('input'));document.querySelector('#aclSave').click()`);
 if(!await js(`document.querySelector('#aclText').getAttribute('aria-invalid')==='true' && document.activeElement.id==='aclText' && document.querySelector('#aclMsg').textContent.includes('Invalid JSON')`))throw new Error('policy JSON recovery lost field');
 await js(`document.querySelector('#aclText').value='[]';document.querySelector('#aclText').dispatchEvent(new Event('input'));`);
 await js(`(async()=>{await load();})()`);
 if(!await js(`document.querySelector('#aclText').value==='[]' && aclDirty && document.querySelector('#aclNet').value===policyNet`))throw new Error('dashboard refresh overwrote unsaved policy');
 await js(`window.policyConfirm=window.confirm;window.confirm=()=>false;document.querySelector('#aclReload').click()`);
 if(!await js(`aclDirty && document.querySelector('#aclText').value==='[]'`))throw new Error('cancelled policy reload discarded draft');
 await js(`window.confirm=()=>true;document.querySelector('#aclReload').click();window.confirm=policyConfirm`);
 await until(()=>js(`!document.querySelector('#aclText').disabled && !aclDirty`),'explicit policy reload');
 await js(`window.fetch=(input,options)=>String(input).startsWith('/admin/api/acl?')?Promise.resolve(new Response('{}',{status:503})):policyFetch(input,options);document.querySelector('#aclReload').click()`);
 await until(()=>js(`document.querySelector('#aclMsg').textContent.includes('Could not read')`),'failed policy read');
 if(!await js(`!document.querySelector('#aclText').value && document.querySelector('#aclText').disabled && document.querySelector('#aclSave').disabled`))throw new Error('unread policy was editable or stale');
 await js(`window.fetch=(input,options)=>String(input).startsWith('/admin/api/acl?')?Promise.resolve(new Response('{"rules":[{}]}',{status:200})):policyFetch(input,options);document.querySelector('#aclReload').click()`);
 await until(()=>js(`document.querySelector('#aclMsg').textContent.includes('Could not read')`),'malformed policy read rejected');
 if(!await js(`document.querySelector('#aclSave').disabled && !document.querySelector('#aclText').value`))throw new Error('malformed policy enabled save');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%';document.querySelector('#aclReload').scrollIntoView({block:'center'})`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('policy reload recovery overflows');
 await shot('policy-read-failed-320',true);
 await cdp('Emulation.setEmulatedMedia',{features:[{name:'forced-colors',value:'active'}]});
 if(!await js(`getComputedStyle(document.querySelector('#aclSave')).borderTopStyle==='solid' && getComputedStyle(document.querySelector('#aclReload')).borderTopStyle==='solid' && document.querySelector('#aclSave').disabled && !document.querySelector('#aclReload').disabled`))throw new Error('forced colors policy recovery loses control states');
 await shot('policy-read-failed-forced-320',true);
 await cdp('Emulation.setEmulatedMedia',{features:[]});

 await cdp('Emulation.clearDeviceMetricsOverride');await js(`document.documentElement.style.fontSize='100%';window.fetch=policyFetch;document.querySelector('#aclReload').click()`);
 await until(()=>js(`!document.querySelector('#aclText').disabled`),'policy retry restored editor');
 await js(`window.policyWrites=0;window.fetch=(input,options)=>input==='/admin/api/acl' && options?.method==='PUT'?(policyWrites++,new Promise(resolve=>window.releasePolicySave=()=>resolve(new Response(JSON.stringify({error:'fixture rejection'}),{status:400})))):policyFetch(input,options);document.querySelector('#aclSave').click();document.querySelector('#aclSave').click()`);
 if(!await js(`policyWrites===1 && document.querySelector('#aclNet').disabled && document.querySelector('#aclText').disabled`))throw new Error('policy pending save target mutable or duplicated');
 await js(`releasePolicySave()`);
 await until(()=>js(`document.querySelector('#aclMsg').textContent.includes('was not saved')`),'server policy rejection');
 if(!await js(`!document.querySelector('#aclSave').disabled && !document.querySelector('#aclNet').disabled`))throw new Error('rejected policy cannot recover');
 await js(`window.fetch=async(input,options)=>{if(input==='/admin/api/acl' && options?.method==='PUT'){const r=await policyFetch(input,options);if(!r.ok)throw new Error('fixture policy did not save');throw new Error('fixture lost save response');}return policyFetch(input,options);};document.querySelector('#aclSave').click()`);
 await until(()=>js(`document.querySelector('#aclMsg').textContent.includes('Could not confirm policy save')`),'lost policy save response honest');
 if(!await js(`document.querySelector('#aclSave').disabled && !document.querySelector('#aclReload').disabled && !document.querySelector('#aclNet').disabled`))throw new Error('uncertain policy save encourages blind retry');
 await js(`window.fetch=policyFetch`);
 await js(`(async()=>{await load();})()`);
 if(!await js(`document.querySelector('#aclSave').disabled && document.querySelector('#aclMsg').textContent.includes('Could not confirm policy save')`))throw new Error('background refresh bypassed uncertain policy inspection');
 await js(`document.querySelector('#aclReload').click()`);
 await until(()=>js(`!document.querySelector('#aclText').disabled && aclTarget===policyNet`),'stored policy inspected after lost response');
 await js(`(async()=>{const other=await api('/admin/api/networks',{name:'policy-other-fixture'});window.policyOther=other.id;await load();})()`);
 if(!await js(`document.querySelector('#aclNet').value===policyNet`))throw new Error('refresh switched policy network');
 await js(`document.querySelector('#keyNet').value=policyOther;document.querySelector('#keyNote').value='unsaved-key-note';`);
 await js(`(async()=>{await load();})()`);
 if(!await js(`document.querySelector('#keyNet').value===policyOther && document.querySelector('#keyNote').value==='unsaved-key-note'`))throw new Error('refresh changed draft key network or note');
 await js(`document.querySelector('#keyNote').value='';document.querySelector('#keyNet').value=policyNet`);

 // Same-network out-of-order reads cannot repaint newer state.
 await js(`window.policyReads=0;window.fetch=(input,options)=>String(input).startsWith('/admin/api/acl?')?new Promise(resolve=>{window['releasePolicyRead'+(++policyReads)]=text=>resolve(new Response(JSON.stringify({rules:JSON.parse(text)}),{status:200}));}):policyFetch(input,options);window.oldPolicyLoad=loadACL();window.newPolicyLoad=loadACL();void 0;`);
 await js(`(async()=>{releasePolicyRead2('[]');await newPolicyLoad;releasePolicyRead1('[{"src":"old-fixture","dst":"*"}]');await oldPolicyLoad;})()`);
 if(!await js(`document.querySelector('#aclText').value==='[]'`))throw new Error('old policy read overwrote newer state');
 await js(`window.fetch=(input,options)=>String(input).startsWith('/admin/api/acl?')?new Promise(resolve=>window.releaseSwitchPolicy=async()=>resolve(await policyFetch(input,options))):policyFetch(input,options);document.querySelector('#aclNet').value=policyOther;document.querySelector('#aclNet').dispatchEvent(new Event('change'))`);
 if(!await js(`!document.querySelector('#aclText').value && document.querySelector('#aclSave').disabled`))throw new Error('network switch retained prior policy');
 await js(`window.fetch=policyFetch;releaseSwitchPolicy()`);
 await until(()=>js(`aclTarget===policyOther && !document.querySelector('#aclText').disabled`),'other policy loaded');
 await js(`document.querySelector('#aclNet').value=policyNet;document.querySelector('#aclNet').dispatchEvent(new Event('change'))`);
 await until(()=>js(`aclTarget===policyNet && !document.querySelector('#aclText').disabled`),'original policy reselected');
 await js(`document.querySelector('#aclText').value='[]';document.querySelector('#aclText').dispatchEvent(new Event('input'));window.confirm=()=>false;document.querySelector('#aclNet').value=policyOther;document.querySelector('#aclNet').dispatchEvent(new Event('change'));window.confirm=policyConfirm`);
 if(!await js(`aclDirty && document.querySelector('#aclNet').value===policyNet`))throw new Error('cancelled policy switch discarded draft');
 await js(`window.confirm=()=>true;document.querySelector('#aclReload').click();window.confirm=policyConfirm`);
 await until(()=>js(`!document.querySelector('#aclText').disabled && !aclDirty`),'policy clean after cancel fixture');

 await js(`window.priorRenderState=state;state={...state,nodes:[{id:'a11y-approved',name:'Example laptop',ipv4:'100.64.0.11',network_id:policyNet,approved:true},{id:'a11y-pending',name:'<img src=x onerror=alert(1)>',ipv4:'100.64.0.12',pubkey:'fixture-public-key',network_id:policyNet,approved:false}],auth_keys:[{id:'a11y-key',key:'synthetic-fixture-only',network_id:policyNet,note:'test note',uses:0}]};render()`);
 const rowAX=await cdp('Accessibility.getFullAXTree');
 for(const name of ['Remove Example laptop, 100.64.0.11','Expire key Example laptop, 100.64.0.11','Approve <img src=x onerror=alert(1)>, 100.64.0.12','Delete test note, network fixture']){
  if(!rowAX.nodes.some(n=>n.role?.value==='button' && n.name?.value===name))throw new Error('contextual action name missing: '+name);
 }
 if(!await js(`!document.querySelector('#pending tbody img')`))throw new Error('context label parsed markup');
 await js(`document.querySelector('#nodes tbody button.danger').focus();window.rowScroll=scrollY;render()`);
 if(!await js(`document.activeElement.getAttribute('aria-label')==='Remove Example laptop, 100.64.0.11' && scrollY===rowScroll`))throw new Error('table refresh lost action focus or moved viewport');
 await js(`document.querySelector('#nodeSection').scrollIntoView({block:'start'})`);
 await shot('operator-records-desktop',true);
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await js(`document.activeElement.scrollIntoView({block:'center',inline:'end'})`);
 if(!await js(`Array.from(document.querySelectorAll('#nodes .row-actions button')).every(button=>button.getBoundingClientRect().width<=document.querySelector('#nodes').clientWidth && button.getBoundingClientRect().height>=44)`))throw new Error('table actions fragment words on narrow screens');
 await shot('operator-device-action-focus',true);
 if(!await js(`document.querySelector('#nodes').scrollWidth<=document.querySelector('#nodes').clientWidth && Array.from(document.querySelectorAll('#nodes tbody td')).every(cell=>cell.classList.contains('row-actions')||getComputedStyle(cell.querySelector('.record-label')).display==='block')`))throw new Error('mobile records retain hidden side columns or lose field labels');
 for(const table of ['pending','keys']){
  if(process.env.MNET_DASHBOARD_SHOTS){const clip=await js(`(()=>{const r=document.querySelector('#${table} tbody tr').getBoundingClientRect();return {x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1};})()`);const image=await cdp('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip});writeFileSync(join(process.env.MNET_DASHBOARD_SHOTS,'operator-'+table+'-record-390.png'),Buffer.from(image.data,'base64'));}
 }
 const mobileRecordAX=await cdp('Accessibility.getFullAXTree');
 for(const tableName of ['Devices','Auth keys','Pending device approvals'])if(!mobileRecordAX.nodes.some(node=>node.role?.value==='table'&&node.name?.value===tableName))throw new Error('responsive record lost table accessibility role');
 for(const [width,size,name] of [[390,'100%','operator-device-record-390'],[320,'200%','operator-device-record-320-large']]){
  await cdp('Emulation.setDeviceMetricsOverride',{width,height:844,deviceScaleFactor:1,mobile:true});
  await js(`document.documentElement.style.fontSize='${size}';document.querySelector('#nodes tbody tr').scrollIntoView({block:'start'})`);
  if(!await js(`document.documentElement.scrollWidth<=innerWidth && Array.from(document.querySelectorAll('#nodes,#pending,#keys')).every(table=>table.scrollWidth<=table.clientWidth)`))throw new Error('mobile records overflow');
  if(process.env.MNET_DASHBOARD_SHOTS){const clip=await js(`(()=>{const r=document.querySelector('#nodes tbody tr').getBoundingClientRect();return {x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1};})()`);const image=await cdp('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip});writeFileSync(join(process.env.MNET_DASHBOARD_SHOTS,name+'.png'),Buffer.from(image.data,'base64'));}
 }
 await js(`document.documentElement.style.fontSize='100%'`);

 await cdp('Emulation.clearDeviceMetricsOverride');
 await until(()=>js(`innerWidth>600 && getComputedStyle(document.querySelector('#nodes tbody tr')).display==='table-row' && getComputedStyle(document.querySelector('#nodes .record-label')).display==='none'`),'desktop records restored after viewport resize');
 await js(`state={...state,nodes:state.nodes.filter(node=>!node.approved)};render()`);
 if(!await js(`document.activeElement.id==='nodes'`))throw new Error('removed row left keyboard focus in detached action');

 // IDs remain data in row IDs, option values and action listeners, including quote/markup-like strings.
 await js(`window.idRenderState=state;window.idAction=runRowAction;window.idInjected=false;window.idCalls=[];window.oddID="fixture');window.idInjected=true;//\\\" data-id-break=\\\"1";runRowAction=(kind,id)=>{idCalls.push({kind,id});};state={...state,networks:[...state.networks,{id:oddID,name:'Literal option'}],nodes:[{id:oddID,name:'Literal device',ipv4:'100.64.0.19',pubkey:'fixture-public-key',network_id:policyNet,approved:false},{id:oddID+'-approved',name:'Literal laptop',ipv4:'100.64.0.20',network_id:policyNet,approved:true}],auth_keys:[{id:oddID+'-key',key:'synthetic-fixture-only',note:'Literal key',network_id:policyNet,uses:0}]};render();document.querySelectorAll('#pending button,#nodes button,#keys button').forEach(button=>button.click());`);
 if(!await js(`!idInjected && !document.querySelector('[data-id-break]') && !document.querySelector('#pending [onclick],#nodes [onclick],#keys [onclick]') && document.querySelector('#pending tbody tr').id==='pending-'+oddID && Array.from(document.querySelector('#keyNet').options).some(option=>option.value===oddID) && idCalls.length===5 && idCalls.every(call=>[oddID,oddID+'-approved',oddID+'-key'].includes(call.id))`))throw new Error('returned ID became markup/script or changed action target');
 await js(`runRowAction=idAction;state=idRenderState;render();`);
 // Device actions review the actual current target, lock duplicates and expose rejected/uncertain outcomes.
 await js(`window.rowFetch=fetch;window.rowConfirm=confirm;window.rowFixture={...state,nodes:[{id:'row-approved',name:'Example laptop',ipv4:'100.64.0.11',network_id:policyNet,approved:true},{id:'row-pending',name:'Example tablet',ipv4:'100.64.0.12',pubkey:'fixture-public-key',network_id:policyNet,approved:false}],auth_keys:[{id:'row-key',key:'synthetic-fixture-only',network_id:policyNet,note:'Example join key',uses:0}]};state=rowFixture;render();window.rowWrites=0;window.fetch=(input,options)=>options?.method==='POST'?(rowWrites++,new Promise(resolve=>window.releaseRow=()=>resolve(new Response('{}',{status:400})))):rowFetch(input,options);window.confirm=()=>false;void 0;`);
 for(const [fn,id] of [['approve','row-pending'],['delNode','row-approved'],['expireNode','row-approved'],['delKey','row-key']]){
  await js(`(async()=>{await ${fn}('${id}');})()`);
 }
 if(!await js(`rowWrites===0 && !rowAction`))throw new Error('cancelled device/key action submitted');
 await js(`window.confirm=()=>{state={...state,nodes:[]};return true;};void delNode('row-approved');state=rowFixture;render();`);
 if(!await js(`rowWrites===0`))throw new Error('stale confirmation sent a device action');
 for(const [fn,id] of [['approve','row-pending'],['delNode','row-approved'],['expireNode','row-approved'],['delKey','row-key']]){
  const before=await js(`rowWrites`);
  await js(`window.confirm=text=>{window.rowReview=text;return true;};window.pendingRow=${fn}('${id}');void ${fn}('${id}');render();`);
  if(!await js(`rowWrites===${before+1} && rowAction && Array.from(document.querySelectorAll('#nodes button,#pending button,#keys button')).every(button=>button.disabled) && rowReview.includes('network fixture')`))throw new Error('pending row action duplicated, lost target context or unlocked after render');
  await js(`(async()=>{releaseRow();await pendingRow;})()`);
  if(!await js(`!rowAction && document.querySelector('#rowActionMsg').textContent.includes('was rejected') && document.activeElement.id==='rowActionMsg'`))throw new Error('row action rejection has no focusable recovery');
 }
 // Failed sign-out must not leave an unresolved row operation permanently pending after it settles.
 await js(`window.rowAlert=alert;window.alert=()=>{};window.fetch=(input,options)=>input==='/admin/api/nodes/expire'?new Promise(resolve=>window.releaseFailedLogoutRow=()=>resolve(new Response('{}',{status:400}))):input==='/admin/api/logout'?Promise.resolve(new Response('{}',{status:503})):rowFetch(input,options);window.failedLogoutRow=expireNode('row-approved');document.querySelector('#logout').click();void 0;`);
 await until(()=>js(`!signingOut && rowAction`),'failed sign-out retained unresolved row operation');
 await js(`(async()=>{releaseFailedLogoutRow();await failedLogoutRow;})()`);
 if(!await js(`rowAction && !document.querySelector('#rowActionInspect').classList.contains('hidden') && document.querySelector('#rowActionMsg').textContent.includes('sign-out attempt')`))throw new Error('failed sign-out left row operation without inspection route');
 await js(`window.alert=rowAlert;window.fetch=rowFetch;document.querySelector('#rowActionInspect').click()`);
 await until(()=>js(`!rowAction`),'failed-sign-out row inspected');
 await js(`state=rowFixture;render();`);
 await js(`window.fetch=()=>Promise.reject(new TypeError('fixture offline'));window.pendingRow=expireNode('row-approved');`);
 await js(`pendingRow`);
 if(!await js(`rowAction && document.querySelector('#rowActionMsg').textContent.includes('Could not confirm') && !document.querySelector('#rowActionInspect').classList.contains('hidden')`))throw new Error('uncertain row action falsely confirmed or allows repeat');
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await js(`document.querySelector('#rowActionPanel').scrollIntoView({block:'center'})`);
 await shot('operator-device-action-uncertain',true);
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%';document.querySelector('#rowActionInspect').scrollIntoView({block:'center'})`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('row action recovery overflows enlarged text');
 await shot('operator-device-action-uncertain-320',true);
 await js(`document.documentElement.style.fontSize='100%'`);
 await cdp('Emulation.clearDeviceMetricsOverride');
 await js(`window.fetch=(input,options)=>input==='/admin/api/state'?Promise.resolve(new Response('{}',{status:503})):rowFetch(input,options);document.querySelector('#rowActionInspect').click()`);
 await until(()=>js(`document.querySelector('#rowActionMsg').textContent.includes('still unconfirmed')`),'failed row inspection');
 if(!await js(`rowAction && !document.querySelector('#rowActionInspect').disabled`))throw new Error('failed inspection unlocked uncertain row action');
 await js(`window.fetch=rowFetch;`);
 await js(`(async()=>{await load();})()`);
 if(!await js(`rowAction`))throw new Error('background refresh bypassed uncertain row action inspection');
 await js(`document.querySelector('#rowActionInspect').click()`);
 await until(()=>js(`!rowAction && document.querySelector('#rowActionMsg').textContent.includes('List refreshed for inspection')`),'explicit row inspection');
 await js(`window.confirm=rowConfirm;clearRowAction();`);
 await js(`state=priorRenderState;render()`);

 // Two state refreshes finish in reverse order. Older data cannot replace newer UI.
 await js(`window.stateReviewFetch=fetch;window.stateReviewBase=state;window.stateReviewReleases=[];window.fetch=(input,options)=>input==='/admin/api/state'?new Promise(resolve=>stateReviewReleases.push(data=>resolve(new Response(JSON.stringify(data),{status:200})))):stateReviewFetch(input,options);window.oldStateReview=load();window.newStateReview=load();void 0;`);
 await js(`(async()=>{stateReviewReleases[1]({...stateReviewBase,reviewMarker:'latest'});await newStateReview;stateReviewReleases[0]({...stateReviewBase,reviewMarker:'stale'});await oldStateReview;window.fetch=stateReviewFetch;})()`);
 if(!await js(`state.reviewMarker==='latest'`))throw new Error('older state refresh replaced latest dashboard');

 // An older failed read cannot mark a newer successful refresh stale.
 await js(`window.reverseFailureFetch=fetch;window.fetch=(input,options)=>input==='/admin/api/state'?new Promise((resolve,reject)=>window.rejectOlderState=reject):reverseFailureFetch(input,options);window.olderFailedState=load();void 0;`);
 await js(`(async()=>{window.fetch=reverseFailureFetch;await load();rejectOlderState(new TypeError('fixture delayed failure'));await olderFailedState;})()`);
 if(!await js(`stateFresh && !document.querySelector('#dashboardControls').disabled && document.querySelector('#stateReadFeedback').classList.contains('hidden')`))throw new Error('older failed read marked new dashboard stale');
 // A failed or malformed latest dashboard read visibly marks old data and pauses changes without losing drafts.
 await js(`window.staleFetch=fetch;window.staleState=state;window.stalePolicy=document.querySelector('#aclText').value;window.staleDirty=aclDirty;document.querySelector('#netName').value='retained refresh draft';document.querySelector('#aclText').value='[]';document.querySelector('#aclText').dispatchEvent(new Event('input'));`);
 for(const bad of ['null','{}','{"networks":{},"nodes":[],"auth_keys":[],"online_by_node":{}}','{"networks":[null],"nodes":[],"auth_keys":[],"online_by_node":{}}','{"networks":[],"nodes":[{}],"auth_keys":[],"online_by_node":{}}','{"networks":[],"nodes":[],"auth_keys":[{}],"online_by_node":{}}','{"networks":[],"nodes":[],"auth_keys":[],"online_by_node":{"example":"connected"}}']){
  await js(`(async()=>{window.fetch=(input,options)=>input==='/admin/api/state'?Promise.resolve(new Response(${JSON.stringify(bad)},{status:200})):staleFetch(input,options);await load();})()`);
  if(!await js(`state===staleState && !stateFresh && document.querySelector('#dashboardControls').disabled && !document.querySelector('#stateReadFeedback').classList.contains('hidden') && document.querySelector('#netName').value==='retained refresh draft' && document.querySelector('#aclText').value==='[]' && aclDirty`))throw new Error('malformed dashboard replaced state or discarded drafts or allowed stale changes');
 }
 await js(`(async()=>{window.fetch=(input,options)=>input==='/admin/api/state'?Promise.reject(new TypeError('fixture offline')):staleFetch(input,options);await load();})()`);
 if(!await js(`document.querySelector('#stateReadMsg').textContent.includes('out of date') && document.querySelector('#newNet button').matches(':disabled')`))throw new Error('connection failure silently retained actionable stale state');
 await js(`window.staleWrites=0;window.fetch=(input,options)=>options?.method==='POST'?(staleWrites++,Promise.resolve(new Response('{}',{status:200}))):staleFetch(input,options);`);
 await js(`(async()=>{await createFromForm('newNet','/admin/api/networks',{name:'must-not-send'});})()`);
 if(!await js(`staleWrites===0`))throw new Error('stale create state guard bypassed');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%';document.querySelector('#stateReadFeedback').scrollIntoView({block:'start'})`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('stale-read recovery overflows enlarged text');
 await shot('operator-refresh-unavailable-320',true);
 await js(`document.documentElement.style.fontSize='100%';window.fetch=staleFetch;document.querySelector('#stateReadRetry').focus();document.querySelector('#stateReadRetry').click()`);
 await until(()=>js(`stateFresh && !document.querySelector('#dashboardControls').disabled && document.querySelector('#stateReadFeedback').classList.contains('hidden') && document.activeElement.id==='overviewHeading'`),'explicit dashboard retry restores current controls/focus');
 if(!await js(`document.querySelector('#netName').value==='retained refresh draft' && document.querySelector('#aclText').value==='[]' && aclDirty`))throw new Error('successful state retry lost unsaved drafts');
 await js(`document.querySelector('#netName').value='';document.querySelector('#aclText').value=stalePolicy;aclDirty=staleDirty;`);
 await cdp('Emulation.clearDeviceMetricsOverride');
 // Create forms: explicit actions on this disposable service only.
 await js(`document.querySelector('#netName').value='   ';document.querySelector('#newNet').dispatchEvent(new Event('submit',{cancelable:true}))`);
 if(!await js(`document.querySelector('#newNetMsg').textContent==='Enter a network name.' && document.activeElement.id==='netName'`))throw new Error('blank network recovery failed');
 await js(`window.createFormFetch=fetch;window.createNetCalls=0;window.fetch=(input,options)=>input==='/admin/api/networks' && options?.method==='POST'?(createNetCalls++,new Promise(resolve=>window.rejectCreateNet=()=>resolve(new Response(JSON.stringify({error:'fixture rejected network'}),{status:400})))):createFormFetch(input,options);document.querySelector('#netName').value='fixture-create-network';document.querySelector('#newNet').requestSubmit();document.querySelector('#newNet').dispatchEvent(new Event('submit',{cancelable:true}))`);
 if(!await js(`createNetCalls===1 && document.querySelector('#netName').disabled`))throw new Error('network create repeated or editable while pending');
 await js(`rejectCreateNet()`);
 await until(()=>js(`!document.querySelector('#netName').disabled && document.querySelector('#newNetMsg').textContent.includes('Not created')`),'network rejection preserves retry');
 if(!await js(`document.querySelector('#netName').value==='fixture-create-network'`))throw new Error('network rejection lost draft');
 await js(`window.fetch=async(input,options)=>{if(input==='/admin/api/networks' && options?.method==='POST'){const response=await createFormFetch(input,options);if(!response.ok)throw new Error('fixture failed');throw new Error('fixture lost network response');}return createFormFetch(input,options);};document.querySelector('#newNet').requestSubmit()`);
 await until(()=>js(`document.querySelector('#newNetMsg').textContent.includes('Could not confirm')`),'network lost response uncertain');
 await js(`window.fetch=createFormFetch`);
 await js(`(async()=>{await load();})()`);
 if(!await js(`document.querySelector('#nets tbody').textContent.includes('fixture-create-network') && document.querySelector('#netName').disabled && document.querySelector('#newNetMsg').textContent.includes('Could not confirm')`))throw new Error('network outcome inspection or duplicate guard failed');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%';document.querySelector('#newNetMsg').scrollIntoView({block:'start'})`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('network outcome overflows enlarged text');
 await shot('network-create-uncertain-320',true);
 await js(`document.querySelector('#newNetInspect').click()`);
 await until(()=>js(`document.activeElement.id==='nets'`),'direct creation result inspection');
 if(!await js(`createStates.newNet.uncertain && document.querySelector('#netName').disabled`))throw new Error('inspection re-enabled uncertain creation');

 await cdp('Emulation.clearDeviceMetricsOverride');await js(`document.documentElement.style.fontSize='100%'`);
 await js(`document.querySelector('#keyTTL').value='1.5';document.querySelector('#newKey').dispatchEvent(new Event('submit',{cancelable:true}))`);
 if(!await js(`document.querySelector('#newKeyMsg').textContent.includes('whole number') && document.activeElement.id==='keyTTL'`))throw new Error('fractional key lifetime silently truncated');
 await js(`window.createKeyCalls=0;window.fetch=(input,options)=>input==='/admin/api/authkeys' && options?.method==='POST'?(createKeyCalls++,new Promise(resolve=>window.rejectCreateKey=()=>resolve(new Response(JSON.stringify({error:'fixture rejected key'}),{status:400})))):createFormFetch(input,options);document.querySelector('#keyTTL').value='1';document.querySelector('#keyNote').value='fixture key draft';document.querySelector('#newKey').requestSubmit();document.querySelector('#newKey').dispatchEvent(new Event('submit',{cancelable:true}))`);
 if(!await js(`createKeyCalls===1 && document.querySelector('#keyNet').disabled && document.querySelector('#keyNote').disabled`))throw new Error('key creation duplicated or target editable');
 await js(`rejectCreateKey()`);
 await until(()=>js(`!document.querySelector('#keyNet').disabled && document.querySelector('#newKeyMsg').textContent.includes('Not created')`),'key rejection retry');
 if(!await js(`document.querySelector('#keyNote').value==='fixture key draft' && document.querySelector('#keyTTL').value==='1'`))throw new Error('key rejection lost draft');
 await js(`window.fetch=createFormFetch;document.querySelector('#newKey').requestSubmit()`);
 await until(()=>js(`!document.querySelector('#keyNet').disabled && document.querySelector('#newKeyMsg').textContent.includes('Auth key created') && document.querySelector('#keys tbody').textContent.includes('fixture key draft')`),'actual key creation table outcome');
 if(!await js(`!document.querySelector('#keyNote').value && !document.querySelector('#keyTTL').value`))throw new Error('confirmed key creation did not clear submitted draft');

 // Commit a disposable key deletion, then drop its response. Inspection reads, never repeats the write.
 await js(`window.actionKey=state.auth_keys.find(key=>key.note==='fixture key draft');window.actionKeyWrites=0;window.confirm=()=>true;window.fetch=async(input,options)=>{if(input==='/admin/api/authkeys/delete'){actionKeyWrites++;const response=await createFormFetch(input,options);if(!response.ok)throw Error('fixture key delete failed');throw Error('fixture lost delete response');}return createFormFetch(input,options);};void delKey(actionKey.id);`);
 await until(()=>js(`rowAction && document.querySelector('#rowActionMsg').textContent.includes('Could not confirm')`),'committed key deletion uncertain');
 await js(`window.fetch=createFormFetch;document.querySelector('#rowActionInspect').click()`);
 await until(()=>js(`!rowAction && !(state.auth_keys||[]).some(key=>key.id===actionKey.id)`),'committed key deletion inspected');
 if(!await js(`actionKeyWrites===1`))throw new Error('inspection repeated key deletion');
 await js(`window.confirm=rowConfirm;clearRowAction();`);
 // Explicit manual grant setup: no email/approval inference, review before write.
 await js(`window.grantNet=document.querySelector('#memberNet').value;document.querySelector('#grantDetails').open=true;document.querySelector('#grantIssuer').value='https://grant-ui.example';document.querySelector('#grantSubject').value='manual-subject';document.querySelector('#grantVerified').checked=true;window.grantConfirm=window.confirm;window.confirm=()=>false;document.querySelector('#memberGrant').requestSubmit()`);
 if(!await js(`(async()=>{const list=await api('/admin/api/memberships?network_id='+encodeURIComponent(grantNet));return list.memberships.length===0;})()`))throw new Error('cancelled grant mutated membership');
 await shot('dashboard-grant-review');
 await js(`document.querySelector('#grantVerified').checked=false;document.querySelector('#memberGrant').dispatchEvent(new Event('submit',{cancelable:true}));`);
 if(!await js(`(async()=>{const list=await api('/admin/api/memberships?network_id='+encodeURIComponent(grantNet));return list.memberships.length===0;})()`))throw new Error('unchecked principal proof created a grant');
 await js(`document.querySelector('#grantVerified').checked=true;window.confirm=text=>{window.grantReview=text;return true;};document.querySelector('#memberGrant').requestSubmit()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('manual-subject')`),'actual UI explicit grant');
 if(!await js(`grantReview.includes(grantNet) && grantReview.includes('https://grant-ui.example') && grantReview.includes('manual-subject') && !document.querySelector('#grantIssuer').value && !document.querySelector('#grantSubject').value && !document.querySelector('#grantVerified').checked`))throw new Error('grant review target/private cleanup wrong');
 await js(`(async()=>{window.confirm=grantConfirm;await api('/admin/api/memberships',{network_id:grantNet,issuer:'https://grant-ui.example',subject:'manual-subject',allow:false});await loadMembers();})()`);
 // A confirmation invalidated by a network change cannot create a grant.
 await js(`(async()=>{document.querySelector('#grantIssuer').value='https://grant-ui.example';document.querySelector('#grantSubject').value='stale-confirmation';document.querySelector('#grantVerified').checked=true;window.confirm=()=>{document.querySelector('#memberNet').dispatchEvent(new Event('change'));return true;};document.querySelector('#memberGrant').dispatchEvent(new Event('submit',{cancelable:true}));window.confirm=grantConfirm;const list=await api('/admin/api/memberships?network_id='+encodeURIComponent(grantNet));if(list.memberships.length || document.querySelector('#grantIssuer').value || document.querySelector('#grantSubject').value || document.querySelector('#grantVerified').checked)throw new Error('invalidated grant confirmation wrote or kept private input');})()`);
 // Commit the real grant but drop its response. Inspect, never blindly retry.
 await js(`window.grantFetch=window.fetch;window.confirm=()=>true;window.fetch=async(input,options)=>{if(input==='/admin/api/memberships' && options?.method==='POST'){const response=await grantFetch(input,options);if(!response.ok)throw new Error('fixture grant failed');throw new Error('fixture lost committed grant response');}return grantFetch(input,options);};document.querySelector('#grantIssuer').value='https://grant-ui.example';document.querySelector('#grantSubject').value='lost-grant-response';document.querySelector('#grantVerified').checked=true;document.querySelector('#memberGrant').dispatchEvent(new Event('submit',{cancelable:true}))`);
 await until(()=>js(`document.querySelector('#memberMsg').textContent==='Could not confirm grant. Inspect grants again before retrying.'`),'lost committed grant response kept uncertain');
 await js(`window.fetch=grantFetch;window.confirm=grantConfirm;document.querySelector('#memberRead').click()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('lost-grant-response')`),'actual inspection settled committed grant');
 await js(`(async()=>{await api('/admin/api/memberships',{network_id:grantNet,issuer:'https://grant-ui.example',subject:'lost-grant-response',allow:false});await loadMembers();})()`);
 // Create an explicit private grant through the actual guarded operator API.
 await js(`(async()=>{window.memberNetwork=document.querySelector('#memberNet').value;await api('/admin/api/memberships',{network_id:memberNetwork,issuer:'https://issuer.example',subject:'<img src=x onerror=alert(1)>',allow:true});document.querySelector('#memberRead').click();})()`);
 await until(()=>js(`document.querySelector('#members tbody').textContent.includes('<img src=x onerror=alert(1)>')`),'actual explicit grant rendered');
 if(!await js(`document.querySelectorAll('#members tbody tr').length===1 && !document.querySelector('#members tbody img')`))throw new Error('private grant did not render as text');
 const memberAX=await cdp('Accessibility.getFullAXTree');
 if(!memberAX.nodes.some(n=>n.role?.value==='button' && n.name?.value==='Remove membership for <img src=x onerror=alert(1)>, issuer https://issuer.example'))throw new Error('membership action lacks contextual accessible name');
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
 if(!await js(`!document.querySelector('#members tbody').textContent && !document.querySelector('#memberMsg').textContent && !document.querySelector('#grantIssuer').value && !document.querySelector('#grantSubject').value && !document.querySelector('#grantVerified').checked && !document.querySelector('#grantDetails').open`))throw new Error('late result crossed network selection');
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
 // Key creation can commit even if only its response is lost.
 await js(`window.keyLostFetch=fetch;window.fetch=async(input,options)=>{if(input==='/admin/api/authkeys' && options?.method==='POST'){const response=await keyLostFetch(input,options);if(!response.ok)throw new Error('fixture key creation failed');throw new Error('fixture dropped key response');}return keyLostFetch(input,options);};document.querySelector('#keyNote').value='lost-key-fixture';document.querySelector('#keyTTL').value='1';document.querySelector('#newKey').requestSubmit()`);
 await until(()=>js(`document.querySelector('#newKeyMsg').textContent.includes('Could not confirm')`),'lost key creation response honest');
 await js(`window.fetch=keyLostFetch`);await js(`(async()=>{await load();})()`);
 if(!await js(`document.querySelector('#keys tbody').textContent.includes('lost-key-fixture') && document.querySelector('#keyNet').disabled && document.querySelector('#newKeyMsg').textContent.includes('Could not confirm')`))throw new Error('key create inspection bypassed uncertain duplicate guard');
 // A device/key action completion must not restore private feedback after logout.
 await js(`window.rowLogoutFetch=window.fetch;window.rowLogoutKey=state.auth_keys.find(key=>key.note==='lost-key-fixture');window.confirm=()=>true;window.fetch=(input,options)=>input==='/admin/api/authkeys/delete'?new Promise(resolve=>window.releaseRowLogout=()=>resolve(new Response('{}',{status:200}))):rowLogoutFetch(input,options);window.rowLogoutPending=delKey(rowLogoutKey.id);void 0;`);
 if(!await js(`rowAction && document.querySelector('#keys button').disabled`))throw new Error('row logout fixture not pending');
 // A late policy save result cannot restore editor state after logout.
 await js(`window.policyLogoutFetch=window.fetch;window.fetch=(input,options)=>input==='/admin/api/acl' && options?.method==='PUT'?new Promise(resolve=>window.releaseLogoutPolicy=()=>resolve(new Response('{"status":"saved"}',{status:200}))):policyLogoutFetch(input,options);document.querySelector('#aclSave').click()`);
 if(!await js(`aclSaving && document.querySelector('#aclSave').disabled`))throw new Error('policy logout fixture not pending');
 // Hold a valid membership response across logout as well as the state refresh.
 await js(`(async()=>{window.memberLogoutFetch=window.fetch;window.fetch=(input,options)=>String(input).startsWith('/admin/api/memberships?')?new Promise(resolve=>{window.releaseMemberLogout=()=>resolve(new Response(memberBody,{status:200,headers:{'Content-Type':'application/json'}}));}):memberLogoutFetch(input,options);window.memberLogoutPending=loadMembers();})()`);
 // Capture a valid old state response and hold a refresh after logout starts.
 await js(`(async()=>{window.savedFetch=window.fetch;window.heldResponse=await (await savedFetch('/admin/api/state')).text();window.releaseHeld=null;window.fetch=(input,options)=>input==='/admin/api/state'?new Promise(resolve=>{window.releaseHeld=()=>resolve(new Response(heldResponse,{status:200,headers:{'Content-Type':'application/json'}}));}):savedFetch(input,options);window.pendingRefresh=load();})()`);
 await js(`document.querySelector('#grantIssuer').value='https://private-draft.example';document.querySelector('#grantSubject').value='private-draft-subject';document.querySelector('#grantVerified').checked=true;document.querySelector('#grantDetails').open=true;document.querySelector('#logout').click()`);
 await until(()=>js(`document.querySelector('#loginMsg').textContent==='Signed out.'`),'logout complete');
 await js(`(async()=>{window.releaseHeld();await window.pendingRefresh;window.fetch=window.savedFetch;window.releaseMemberLogout();await window.memberLogoutPending;window.fetch=memberLogoutFetch;})()`);
 if(!await js(`state===null && getComputedStyle(document.querySelector('#app')).display==='none' && getComputedStyle(document.querySelector('#logout')).display==='none' && !document.querySelector('#nets tbody').textContent && !document.querySelector('#keys tbody').textContent && !document.querySelector('#keyNet').textContent && !document.querySelector('#memberNet').textContent && !document.querySelector('#members tbody').textContent && !document.querySelector('#memberMsg').textContent && !document.querySelector('#grantIssuer').value && !document.querySelector('#grantSubject').value && !document.querySelector('#grantVerified').checked && !document.querySelector('#grantDetails').open`))throw new Error('late refresh restored private dashboard state');
 await js(`(async()=>{releaseRowLogout();await rowLogoutPending;})()`);
 if(!await js(`!rowAction && document.querySelector('#rowActionPanel').classList.contains('hidden') && !document.querySelector('#rowActionMsg').textContent`))throw new Error('late row action restored private feedback after logout');
 await js(`releaseLogoutPolicy()`);
 await until(()=>js(`!aclSaving`),'policy state invalidated at logout');
 if(!await js(`!document.querySelector('#aclText').value && !document.querySelector('#aclMsg').textContent && document.querySelector('#aclText').disabled && document.querySelector('#aclSave').disabled`))throw new Error('late policy save repainted signed-out UI');
 if(!await js(`!document.querySelector('#newNetMsg').textContent && !document.querySelector('#newKeyMsg').textContent && !document.querySelector('#netName').value && !document.querySelector('#keyNote').value && !document.querySelector('#keyTTL').value && !createStates.newNet.uncertain && !createStates.newKey.uncertain`))throw new Error('create drafts/outcome leaked across sign-out');
 const replay=await fetch(base+'/admin/api/state',{headers:{Cookie:'mnet_admin='+token}});if(replay.status!==401)throw new Error('logged-out cookie replay accepted');
 const remaining=(await cdp('Network.getCookies',{urls:[base]})).cookies;if(remaining.some(c=>c.name==='mnet_admin'))throw new Error('logout cookie not cleared');
 if(!await js(`document.activeElement.id==='pw'`))throw new Error('sign-out did not restore login keyboard focus');
 await shot('dashboard-signed-out');
 await cdp('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'dark'}]});
 if(!await js(`matchMedia('(prefers-color-scheme: dark)').matches && getComputedStyle(document.documentElement).colorScheme==='light' && getComputedStyle(document.body).color==='rgb(29, 29, 31)' && getComputedStyle(document.querySelector('#pw')).backgroundColor==='rgb(255, 255, 255)'`))throw new Error('supported light palette changed under dark system preference');
 await cdp('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await js(`document.querySelector('#pw').scrollIntoView({block:'center'})`);
 await shot('operator-system-dark-light-ui',true);
 await cdp('Emulation.setEmulatedMedia',{features:[{name:'forced-colors',value:'active'}]});
 if(!await js(`matchMedia('(forced-colors: active)').matches`))throw new Error('forced colors emulation not active');
 if(!await js(`getComputedStyle(document.querySelector('#loginBtn')).borderTopStyle==='solid' && parseFloat(getComputedStyle(document.querySelector('#loginBtn')).borderTopWidth)>=1`))throw new Error('forced-colors action boundary missing');
 await shot('operator-forced-colors',true);

 await cdp('Emulation.setEmulatedMedia',{features:[]});
 await cdp('Emulation.clearDeviceMetricsOverride');

 // The public request page guides operators, but does not assert unverified live state.
 await cdp('Page.navigate',{url:base+'/join/approve?node_id=fixture-device-request'});
 await until(()=>js(`document.querySelector('h1')?.textContent==='Review a device request'`),'device request guidance');
 if(!await js(`document.querySelector('code').textContent==='fixture-device-request' && document.querySelector('a.button').getAttribute('href')==='/admin#pending-fixture-device-request' && document.body.textContent.includes('does not check')`))throw new Error('device request changed scope or target');
 await shot('approval-request');
 await cdp('Emulation.setDeviceMetricsOverride',{width:320,height:720,deviceScaleFactor:1,mobile:true});
 await js(`document.documentElement.style.fontSize='200%'`);
 if(!await js(`document.documentElement.scrollWidth<=320`))throw new Error('device request overflows enlarged text');
 await shot('approval-request-320-large');
 await js(`document.querySelector('a.button').scrollIntoView({block:'center'})`);
 await shot('approval-request-320-action',true);
 await cdp('Emulation.clearDeviceMetricsOverride');
 await js(`document.querySelector('a.button').click()`);
 await until(()=>js(`document.querySelector('#loginBtn') && getComputedStyle(document.querySelector('#app')).display==='none'`),'request link still requires operator sign-in');
 const literalID='<img src=x onerror="window.injected=true"> & # ? /';
 await cdp('Page.navigate',{url:base+'/join/approve?node_id='+encodeURIComponent(literalID)});
 await until(()=>js(`document.querySelector('code')`),'literal request ID');
 if(!await js(`!document.querySelector('img') && !window.injected && document.querySelector('code').textContent===${JSON.stringify(literalID)} && decodeURIComponent(new URL(document.querySelector('a.button').href).hash.slice(1))===${JSON.stringify('pending-'+literalID)}`))throw new Error('device request parsed ID as markup or changed fragment');
 await cdp('Page.navigate',{url:base+'/join/approve'});
 await until(()=>js(`document.querySelector('code')?.textContent==='Not supplied'`),'missing device ID');
 if(!await js(`document.querySelector('a.button').getAttribute('href')==='/admin'`))throw new Error('missing device ID fabricated target');

 console.log('PASS real Chrome dashboard: mobile labeled device/pending/key records and desktop table/AX-role preservation, literal ID listeners, failed-sign-out action recovery, initial read recovery, stale/malformed latest-state locking with draft retention and explicit retry, older-failure suppression, reviewed/cancelled/stale device-key actions, pending/rerender/duplicate guards, rejected and uncertain states, failed and explicit inspection, real committed-key-deletion lost-response inspection without repeat, guarded creation/rejection/uncertain-outcome inspection and draft cleanup, key draft-target retention, contextual accessibility-tree action names, row refresh/removal focus retention, policy read/edit/refresh/target/reload/save-uncertainty/logout guards, keyboard/focus transitions, pending-submit guard, rejected-password versus unavailable-service recovery, enlarged retry button, refresh focus retention, login visibility, same-origin sign-in, network/key render, actual scoped membership reads, reviewed manual grant/cancel/unchecked rejection, cancel/confirmed removal and refresh, lost-committed-response inspection, stale row/confirmation guard, HTML-as-text, empty grants, failed-read clear/retry, invalid-response handling, same-network out-of-order reads, network-switch/logout late-result suppression, expiry boundaries, sign-out, cleared populated private UI, delayed-refresh suppression and logged-out token replay and non-authorizing device-request guidance');
}finally{ws?.close();await Promise.all([stopChild(chrome),stopChild(control)]);rmSync(dir,{recursive:true,force:true,maxRetries:5,retryDelay:100});}
