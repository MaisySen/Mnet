// Fixed, bounded Linux process-wait evidence. Never return raw kernel symbols,
// process names, argv, file paths, environment, payloads or private strings.
export function chromeWaitCategory(value){
 const key=String(value).trim();
 if(key==='0')return 'not-blocked-or-hidden';
 if(['futex_wait_queue','futex_wait_queue_me','futex_wait','do_futex','futex_wait_multiple'].includes(key))return 'futex';
 if(['do_epoll_wait','ep_poll','ep_poll_callback','do_select','poll_schedule_timeout','poll_schedule_timeout.constprop.0'].includes(key))return 'poll';
 if(['pipe_read','pipe_write','wait_for_partner','anon_pipe_read','anon_pipe_write'].includes(key))return 'pipe';
 if(['hrtimer_nanosleep','do_nanosleep'].includes(key))return 'sleep';
 if(['io_schedule','folio_wait_bit_common','wait_on_page_bit_common','wait_on_page_bit','wait_on_buffer','wait_on_page_writeback','balance_dirty_pages','rq_qos_wait','blk_mq_get_tag','blk_mq_get_tags'].includes(key))return 'io-wait';
 return 'other-or-unavailable';
}
export function chromeWaitSnapshot(pid,read){
 if(!Number.isSafeInteger(pid)||pid<1||typeof read!=='function')return 'mainWait=other-or-unavailable childWaitSamples=0 childWaitUnavailable=0 childWaitIO=0 childWaitFutex=0 childWaitPoll=0 childWaitPipe=0 childWaitSleep=0 childWaitHidden=0 childWaitOther=0 childWaitTruncated=false';
 const take=(path,limit)=>{try{const value=read(path,limit);return typeof value==='string'&&value.length<=limit?value:'';}catch{return '';}};
 const main=chromeWaitCategory(take(`/proc/${pid}/wchan`,256));
 const children=take(`/proc/${pid}/task/${pid}/children`,4096).trim();
 const ids=/^(?:[1-9][0-9]{0,9}\s*)*$/.test(children)?children.split(/\s+/).filter(Boolean):[];
 const counts={'io-wait':0,futex:0,poll:0,pipe:0,sleep:0,'not-blocked-or-hidden':0,'other-or-unavailable':0};
 let unavailable=0;
 for(const child of ids.slice(0,32)){
  const raw=take(`/proc/${child}/wchan`,256);if(!raw)unavailable++;
  counts[chromeWaitCategory(raw)]++;
 }
 return `mainWait=${main} childWaitSamples=${Math.min(ids.length,32)} childWaitUnavailable=${unavailable} childWaitIO=${counts['io-wait']} childWaitFutex=${counts.futex} childWaitPoll=${counts.poll} childWaitPipe=${counts.pipe} childWaitSleep=${counts.sleep} childWaitHidden=${counts['not-blocked-or-hidden']} childWaitOther=${counts['other-or-unavailable']} childWaitTruncated=${ids.length>32}`;
}

// Bounded fixed pressure categories only. Never log raw /proc text, kernel
// symbols, machine paths, process identifiers or resource-specific counters.
function pressureCategory(raw){
 if(typeof raw!=='string'||raw.length>2048)return 'unavailable';
 const lines=raw.trim().split('\n');
 if(lines.length<1||lines.length>2)return 'unavailable';
 const seen=new Set();let full=false,some=false;
 for(const line of lines){
  const match=/^(some|full) avg10=([0-9]+\.[0-9]{2}) avg60=([0-9]+\.[0-9]{2}) avg300=([0-9]+\.[0-9]{2}) total=([0-9]+)$/.exec(line);
  if(!match||seen.has(match[1]))return 'unavailable';
  seen.add(match[1]);
  const values=match.slice(2,5).map(Number);
  if(values.some(v=>!Number.isFinite(v)||v<0||v>100))return 'unavailable';
  if(match[1]==='full')full=values[0]>0;
  else some=values[0]>0;
 }
 if(!seen.has('some'))return 'unavailable';
 return full?'full-pressure':some?'some-pressure':'no-recent-pressure';
}
export function chromePressureSnapshot(read){
 const output=[];
 for(const kind of ['cpu','io','memory']){
  let raw;try{raw=typeof read==='function'?read(`/proc/pressure/${kind}`,2048):undefined;}catch{}
  output.push(`${kind}Pressure=${pressureCategory(raw)}`);
 }
 return output.join(' ');
}
