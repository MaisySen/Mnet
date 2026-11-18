// Fixed categories only: never return browser stderr or machine/profile paths.
export function chromeStartupDiagnostics(){
 let tail='',seen=false;
 const flags={sandbox:false,profileLock:false,crash:false,debugPipe:false};
 return {
  feed(chunk){
   seen=true;tail=(tail+String(chunk)).slice(-4096);
   flags.sandbox ||= /no usable sandbox|running as root without --no-sandbox/i.test(tail);
   flags.profileLock ||= /singletonlock|processsingleton|profile.*in use/i.test(tail);
   flags.crash ||= /received signal|fatal:|trace\/breakpoint trap/i.test(tail);
   flags.debugPipe ||= /remote.debugging.pipe.*(?:invalid|not open|error)|file descriptors are not open/i.test(tail);
  },
  summary(){return `stderrSeen=${seen} sandboxError=${flags.sandbox} profileLockError=${flags.profileLock} crashMarker=${flags.crash} debugPipeError=${flags.debugPipe}`;}
 };
}
