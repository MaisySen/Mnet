// Startup evidence only. Never retain or print transport/browser payloads.
export function chromePipeDiagnostics(now=()=>performance.now()){
 const start=now();
 let spawned=false,writes=0,writeCallbacks=0,writeErrors=0,receivedBytes=0,firstResponse='none';
 let sendEnded=false,receiveEnded=false,sendClosed=false,receiveClosed=false;
 const add=(a,b)=>Math.min(1000000000,a+b);
 return {
  spawned(){spawned=true;},
  writing(){writes=add(writes,1);},
  wrote(error){if(error)writeErrors=add(writeErrors,1);else writeCallbacks=add(writeCallbacks,1);},
  received(chunk){receivedBytes=add(receivedBytes,Buffer.byteLength(chunk));if(firstResponse==='none'){const elapsed=now()-start;firstResponse=elapsed<100?'lt100ms':elapsed<1000?'lt1s':elapsed<5000?'lt5s':elapsed<15000?'lt15s':'gte15s';}},
  sendEnded(){sendEnded=true;},receiveEnded(){receiveEnded=true;},
  sendClosed(){sendClosed=true;},receiveClosed(){receiveClosed=true;},
  summary(){return `spawnSeen=${spawned} requestWrites=${writes} writeCallbacks=${writeCallbacks} writeErrors=${writeErrors} receiveBytes=${receivedBytes} firstResponse=${firstResponse} sendEnded=${sendEnded} receiveEnded=${receiveEnded} sendClosed=${sendClosed} receiveClosed=${receiveClosed}`;}
 };
}
export function chromeVersionCategory(value){
 const match=/^Google Chrome(?: for Testing)? (\d{1,4}\.\d{1,4}\.\d{1,6}\.\d{1,6})\s*$/.exec(String(value).trim());
 return match?match[1]:'unavailable';
}
export function chromeProcessState(value){
 // /proc stat comm may contain spaces and parentheses; only inspect the state
 // after its final closing parenthesis, never return the process name.
 const state=/^ ([RSDZTWXIKt]) /.exec(String(value).slice(String(value).lastIndexOf(')')+1));
 return state?({R:'running',S:'sleeping',D:'uninterruptible',Z:'zombie',T:'stopped',t:'tracing',W:'paging',X:'dead',I:'idle',K:'wakekill'}[state[1]]):'unavailable';
}
