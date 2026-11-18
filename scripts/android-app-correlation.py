"""One application of calibrated synthetic joins to disposable app DNS sockets.
Raw trace, socket pointers, source ports, UID and kernel layouts never exported.
"""
import importlib.util
import json
import re
import shlex
import subprocess
from pathlib import Path

def parse(raw):
    events=[]
    for line in raw.splitlines():
        m=re.search(r'-(\d+)\s+\[\d+\].*?\s(\d+\.\d+):\s+(\w+):\s+(.*)',line)
        if not m:continue
        tid,ts,event,body=m.groups();values={}
        for k,v in re.findall(r'(\w+)=(0x[0-9a-fA-F]+|-?\d+|[0-9a-fA-F]+)(?=\s|$)',body):
            if k not in ('sk','skb','sport','dport','mark','uid','length','result','reason','protocol'):continue
            if v.startswith('0x') or k in ('skb','sk'):values[k]=int(v,16)
            elif re.fullmatch(r'-?\d+',v):values[k]=int(v)
        events.append((int(tid),float(ts),event,values))
    return events

class Trace:
    def __init__(self,adb,uids):
        self.adb=[str(x) for x in adb];self.uids=uids;self.hooks=[];self.instance=None
        self.report={'scope':'actual-app-uid-dns','userspace_bind_denial_unverified':True}
    def call(self,args):return subprocess.run(self.adb+args,capture_output=True,timeout=20)
    def shell(self,*args):return self.call(['shell',shlex.join(str(x) for x in args)])
    def write(self,path,value):return self.shell('sh','-c','printf "%s\\n" "$1" > "$2"','probe',value,path).returncode==0
    def start(self):
        spec=importlib.util.spec_from_file_location('native_btf',Path(__file__).with_name('android-btf-layout.py'))
        mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod)
        btf=mod.BTF(self.call(['exec-out','cat','/sys/kernel/btf/vmlinux']).stdout)
        offsets={k:btf.offset(t,p) for k,t,p in (('sport','sock','__sk_common.skc_num'),('dport','sock','__sk_common.skc_dport'),('mark','sock','sk_mark'),('uid','sock','sk_uid.val'),('data','sk_buff','data'))}
        if [p[0] for p in btf.function_parameters('kfree_skb_reason')][:2]!=['skb','reason']:raise ValueError('drop signature')
        self.reasons={v:k for t in btf.types if t and t['kind']==6 and t['name']=='skb_drop_reason' for k,v in t.get('values',{}).items()}
        self.trace='/sys/kernel/tracing';self.instance=self.trace+'/instances/mnet_app_join'
        if self.shell('mkdir',self.instance).returncode:raise RuntimeError('trace instance')
        self.write(self.instance+'/buffer_size_kb','16384')
        self.report['boot_trace_clock_set']=self.write(self.instance+'/trace_clock','boot')
        o=offsets
        specs={
            'udp':'p:kprobes/mnet_app_udp udp_sendmsg sk=%di sport=+'+str(o['sport'])+'(%di):u16 dport=+'+str(o['dport'])+'(%di):u16 mark=+'+str(o['mark'])+'(%di):u32 uid=+'+str(o['uid'])+'(%di):u32 length=%dx:u64',
            'udpret':'r:kprobes/mnet_app_udpret udp_sendmsg result=$retval:s64',
            'security':'r:kprobes/mnet_app_security security_socket_sendmsg result=$retval:s64',
            'sendto':'r:kprobes/mnet_app_sendto __sys_sendto result=$retval:s64',
            'ip':'p:kprobes/mnet_app_ip ip_rcv skb=%di protocol=+9(+'+str(o['data'])+'(%di)):u8 sport=+20(+'+str(o['data'])+'(%di)):u16 dport=+22(+'+str(o['data'])+'(%di)):u16',
            'drop':'p:kprobes/mnet_app_drop kfree_skb_reason skb=%di reason=%si:u32',
        }
        self.report['probes']={}
        for label,decl in specs.items():
            name='mnet_app_'+label
            registered=self.shell('sh','-c','printf "%s\\n" "$1" >> "$2"','probe',decl,self.trace+'/kprobe_events').returncode==0
            enabled=False
            if registered:
                self.hooks.append(name)
                if label=='udp':
                    filt='('+' || '.join('uid == '+str(u) for u in self.uids if u is not None)+') && length < 256'
                    self.report['uid_filter_set']=self.write(self.instance+'/events/kprobes/'+name+'/filter',filt)
                if label=='ip':self.write(self.instance+'/events/kprobes/'+name+'/filter','protocol == 17 && sport == 13568')
                enabled=self.write(self.instance+'/events/kprobes/'+name+'/enable','1')
            self.report['probes'][label]={'registered':registered,'enabled':enabled}
        self.report['trace_started']=self.write(self.instance+'/tracing_on','1')
        self.report['selinux_before_enforcing']=self.shell('getenforce').stdout.strip()==b'Enforcing'
    def finish(self):
        if self.instance is None:return self.report
        try:
            self.write(self.instance+'/tracing_on','0')
            events=parse(self.shell('cat',self.instance+'/trace').stdout.decode(errors='replace'))
            applications=[]
            for label,package in (('owner','au.fintex.mnet'),('independent','au.fintex.mnet.fixture')):
                value=self.call(['exec-out','run-as',package,'cat','files/fixture-dns-sockets.json'])
                try:rows=json.loads(value.stdout)
                except ValueError:rows=[]
                for row in rows[:32]:
                    # Match exact original fixture thread/port, not a random UDP
                    # packet seen during the same test. Closed sockets get their
                    # own time windows, so ephemeral port reuse is bounded.
                    tid=row.get('tid');port=row.get('port');ts=row.get('elapsed_ms',0)/1000
                    entries=[x for x in events if x[0]==tid and x[2]=='mnet_app_udp' and x[3].get('sport')==port and abs(x[1]-ts)<1]
                    output={'app':label,'point':row.get('point') if row.get('point') in ('original-before-network-bind','original-before-send','original-received','contrast-before-send') else 'other','udp_entry_count':len(entries)}
                    if entries:
                        entry=entries[0]
                        following=[x[1] for x in events if x[0]==tid and x[2]=='mnet_app_udp' and x[1]>entry[1]]
                        end=min([entry[1]+0.25]+following)
                        same=[x for x in events if x[0]==tid and entry[1]<=x[1]<end]
                        output['uid_matches_app']=entry[3].get('uid')==self.uids[0 if label=='owner' else 1]
                        output['marked_network_nonzero']=(entry[3].get('mark',0)&65535)!=0
                        output['kernel_udp_returns']=[x[3]['result'] for x in same if x[2]=='mnet_app_udpret' and 'result'in x[3]][:4]
                        output['security_returns']=[x[3]['result'] for x in same if x[2]=='mnet_app_security' and 'result'in x[3]][:4]
                        output['sys_sendto_returns']=[x[3]['result'] for x in same if x[2]=='mnet_app_sendto' and 'result'in x[3]][:4]
                    packets=[x for x in events if x[2]=='mnet_app_ip' and ((x[3].get('dport',0)&255)<<8|x[3].get('dport',0)>>8)==port and ts<=x[1]<=ts+5.5]
                    output['incoming_reply_count']=len(packets);output['reply_drops']=[]
                    for index,p in enumerate(packets):
                        next_ts=packets[index+1][1] if index+1<len(packets) else p[1]+0.1
                        drops=[x for x in events if x[2]=='mnet_app_drop' and p[1]<=x[1]<next_ts and x[3].get('skb')==p[3].get('skb')]
                        output['reply_drops'].append([self.reasons.get(x[3].get('reason'),'UNKNOWN') for x in drops][:4])
                    # A denial before udp_sendmsg still needs syscall return at
                    # the exact recorded thread/time. This does not explain netd.
                    output['nearby_sendto_returns']=[x[3]['result'] for x in events if x[0]==tid and x[2]=='mnet_app_sendto' and ts<=x[1]<=ts+.25 and 'result'in x[3]][:4]
                    applications.append(output)
            self.report['socket_observations']=applications
            stats=self.shell('sh','-c','cat "$1"/per_cpu/cpu*/stats','probe',self.instance).stdout.decode()
            overruns=re.findall(r'^overrun:\s+(\d+)$',stats,re.M)
            self.report['all_cpu_overruns_zero']=bool(overruns) and all(int(x)==0 for x in overruns)
            self.report['selinux_after_enforcing']=self.shell('getenforce').stdout.strip()==b'Enforcing'
        except (OSError,ValueError,RuntimeError,subprocess.TimeoutExpired):self.report['collection_unavailable']=True
        finally:
            self.write(self.instance+'/tracing_on','0');self.write(self.instance+'/events/enable','0');self.shell('rmdir',self.instance)
            for name in self.hooks:self.shell('sh','-c','printf "%s\\n" "$1" >> "$2"','probe','-:kprobes/'+name,self.trace+'/kprobe_events')
        return self.report
