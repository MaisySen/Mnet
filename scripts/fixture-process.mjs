// Local disposable fixture children only. Wait before removing their files.
import {setTimeout as delay} from 'node:timers/promises';
export async function stopChild(child,graceMs=2000){
 if(!child || child.exitCode!==null || child.signalCode!==null)return;
 let closed=false;
 const closedPromise=new Promise(resolve=>child.once('close',()=>{closed=true;resolve();}));
 child.kill('SIGTERM');
 await Promise.race([closedPromise,delay(graceMs)]);
 if(!closed){child.kill('SIGKILL');await Promise.race([closedPromise,delay(graceMs)]);}
 if(!closed)throw new Error('Fixture child did not close within shutdown budget');
}
