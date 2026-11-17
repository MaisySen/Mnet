// Local-only browser verification. Requires Node22+ and Chrome; no user profile is loaded.
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { mkdtemp, readFile, rm, mkdir, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
const here=dirname(fileURLToPath(import.meta.url));
const files=new Map([['/','index.html'],['/index.html','index.html'],['/style.css','style.css'],['/app.mjs','app.mjs'],['/model.mjs','model.mjs']]);
const server=createServer(async(req,res)=>{const file=files.get(req.url);if(!file){res.writeHead(404);res.end();return;}try{res.setHeader('content-type',file.endsWith('.mjs')?'text/javascript':file.endsWith('.css')?'text/css':'text/html');res.end(await readFile(join(here,file)));}catch{res.writeHead(500);res.end();}});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const port=server.address().port;
const profile=await mkdtemp(join(tmpdir(),'mnet-prototype-'));
const env=Object.fromEntries(Object.entries(process.env).filter(([k])=>k!=='LD_PRELOAD'&&!k.startsWith('FAKETIME')));
const chrome=spawn(process.env.CHROME_BIN||'google-chrome',['--headless=new','--no-sandbox','--disable-gpu','--disable-dev-shm-usage','--disable-background-networking','--no-first-run','--remote-debugging-pipe',`--user-data-dir=${profile}`,'about:blank'],{stdio:['ignore','ignore','pipe','pipe','pipe'],env,detached:true});
let next=0,buf='',session,errors=[];const pending=new Map();
chrome.stdio[4].setEncoding('utf8');chrome.stdio[4].on('data',s=>{buf+=s;let i;while((i=buf.indexOf('\0'))>=0){const m=JSON.parse(buf.slice(0,i));buf=buf.slice(i+1);if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);clearTimeout(p.timer);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result);}}else if(m.method==='Runtime.exceptionThrown'){errors.push(m.params.exceptionDetails.text);}}});
function send(method,params={},sid=session){const id=++next;return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout: '+method));},15000);pending.set(id,{resolve,reject,timer});chrome.stdio[3].write(JSON.stringify({id,method,params,...(sid?{sessionId:sid}:{})})+'\0');});}
const evaluate=async expression=>{const r=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.text);return r.result.value;};
const click=async a=>assert.equal(await evaluate(`(()=>{const b=document.querySelector('[data-action="${a}"]');if(!b)return false;b.click();return true;})()`),true,'missing action '+a);
const setup=['SIGN_IN','RETURN','CHOOSE','ADD','ALLOW','CHECK','VERIFY','DONE'];
const request=[...setup,'DETAIL','CONNECT','REQUEST','HOST_REPLY'];
const cases=[
 ['welcome','Owner setup',[],'Your devices. One workspace.'],['signin','Owner setup',['SIGN_IN'],'Continue in your browser'],['workspace','Owner setup',['SIGN_IN','RETURN'],'Where do these devices belong?'],['empty','Owner setup',['SIGN_IN','RETURN','CHOOSE'],'Devices'],['permission','Owner setup',[...setup.slice(0,4)],'Allow a private connection'],['denied','Permission declined',[...setup.slice(0,5)],'Allow a private connection'],['connecting','Owner setup',setup.slice(0,5),'Connecting this device'],['checks','Owner setup',setup.slice(0,6),'Prove the connection'],['ready','Owner setup',setup.slice(0,7),'This device is ready'],['devices','Owner setup',setup,'Devices'],['another','Owner setup',[...setup,'ADD'],'Add another device'],['details','Owner setup',[...setup,'DETAIL'],'Sample laptop'],['request','Owner setup',[...setup,'DETAIL','CONNECT'],'Ask to view Sample laptop'],['waiting','Owner setup',[...setup,'DETAIL','CONNECT','REQUEST'],'Waiting for the host'],['host','Owner setup',request,'Allow this view request?'],['active','Owner setup',[...request,'VIEW'],'Viewing Sample laptop'],['control-consent','Owner setup',[...request,'VIEW','CONTROL_REQUEST'],'Allow input control?'],['control','Owner setup',[...request,'VIEW','CONTROL_REQUEST','HOST_ALLOW'],'Viewing Sample laptop'],['reconnecting','Owner setup',[...request,'VIEW','NETWORK_LOST'],'Reconnecting safely'],['offline','Peer offline',['DETAIL','CONNECT'],'Sample laptop is offline'],['declined','Session declined',request,'The host declined'],['ended','Owner setup',[...request,'VIEW','END'],'Remote access has ended'],['remove','Owner setup',[...setup,'DETAIL','REMOVE'],'Remove Sample laptop?'],['removed','Owner setup',[...setup,'DETAIL','REMOVE','CONFIRM_REMOVE'],'Devices'],['settings','Owner setup',[...setup,'SETTINGS'],'Connection and account'],['blocked','Not eligible',['SIGN_IN','RETURN'],'Remote access is blocked'],['revoked','Owner setup',[...request,'VIEW','MEMBERSHIP_REMOVED'],'Remote access is blocked'],['failed','Connection failed',setup.slice(0,7),'Not connected']
];
let checked=0;
try{
 let target;for(let n=0;n<30;n++){const {targetInfos}=await send('Target.getTargets',{},undefined);target=targetInfos.find(t=>t.type==='page');if(target)break;await new Promise(r=>setTimeout(r,100));}assert.ok(target,'no Chrome tab');
 session=(await send('Target.attachToTarget',{targetId:target.targetId,flatten:true},undefined)).sessionId;
 await send('Page.enable');await send('Runtime.enable');
 for(const width of [320,390,1280]){
  await send('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:false});
  await send('Page.navigate',{url:`http://127.0.0.1:${port}/`});
  for(let n=0;n<50;n++){if(await evaluate('!!document.querySelector("h1")'))break;await new Promise(r=>setTimeout(r,30));}
  for(const [name,scenario,actions,title] of cases){
   await evaluate(`(()=>{const s=document.querySelector('#scenario');s.value=${JSON.stringify(scenario)};s.dispatchEvent(new Event('change'));})()`);
   for(const a of actions)await click(a);
   assert.equal(await evaluate('document.querySelector("h1").textContent'),title,`${width}: ${name}`);
   assert.equal(await evaluate('document.documentElement.scrollWidth <= innerWidth'),true,`${width}: ${name}: horizontal overflow`);
   assert.equal(await evaluate('Array.from(document.querySelectorAll("button")).every(b=>b.getBoundingClientRect().height >=44)'),true,`${width}: ${name}: short touch target`);
   assert.equal(await evaluate('document.querySelector("#scenario").getBoundingClientRect().right + 8 <= document.querySelector("#reset").getBoundingClientRect().left'),true,`${width}: overlapping review controls`);
   assert.equal(await evaluate('document.activeElement.tagName'), 'H1',`${name}: focus not moved to heading`);
   if(process.env.SCREENSHOT_DIR){await mkdir(process.env.SCREENSHOT_DIR,{recursive:true});const {cssContentSize:size}=await send('Page.getLayoutMetrics');const {data}=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width,height:size.height,scale:1}});await writeFile(join(process.env.SCREENSHOT_DIR,`${name}-${width}.png`),Buffer.from(data,'base64'));}
   checked++;
  }
 }
 assert.deepEqual(errors,[],'browser exceptions');console.log(`PASS: ${checked} screen/width checks, no horizontal overflow, minimum44px buttons, heading focus and no browser exceptions.`);
}finally{const exited=new Promise(r=>chrome.once('exit',r));try{process.kill(-chrome.pid,'SIGKILL');}catch{}server.close();await exited;await rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:100});}
