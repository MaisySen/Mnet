"""Synthetic correlation only. Raw pointers/trace/BTF stay private and ephemeral."""
import importlib.util
import json
import re
from pathlib import Path

def load(path,name):
    spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

def run(api,trace,instance,hooks,compiler):
    report={'scope':'synthetic-only','userspace_bind_denial_unverified':True}
    btfdata=api.adb('exec-out','cat','/sys/kernel/btf/vmlinux')
    if btfdata.returncode:report['btf_read_failed']=True;return report
    try:
        btf=load('scripts/android-btf-layout.py','mnet_btf').BTF(btfdata.stdout)
        offsets={name:btf.offset(typ,path) for name,typ,path in (
            ('sport','sock','__sk_common.skc_num'),('dport','sock','__sk_common.skc_dport'),
            ('mark','sock','sk_mark'),('data','sk_buff','data'))}
        report['btf_offsets_resolved']=True
    except (ValueError,KeyError,IndexError):report['btf_offsets_resolved']=False;return report
    built=api.command([compiler,'-Wall','-Wextra','-Werror','-O2','scripts/android-synthetic-correlation.c','-o','/tmp/mnet-correlation'])
    report['helper_compiled']=built.returncode==0
    if built.returncode:return report
    api.adb('push','/tmp/mnet-correlation','/data/local/tmp/mnet-correlation');api.shell('chmod','700','/data/local/tmp/mnet-correlation')
    specs={
        'udp_entry':'p:kprobes/mnet_join_udp udp_sendmsg sk=%di sport=+'+str(offsets['sport'])+'(%di):u16 dport=+'+str(offsets['dport'])+'(%di):u16 mark=+'+str(offsets['mark'])+'(%di):u32',
        'udp_return':'r:kprobes/mnet_join_udpret udp_sendmsg result=$retval:s64',
        'security_return':'r:kprobes/mnet_join_security security_socket_sendmsg result=$retval:s64',
        'tun_return':'r:kprobes/mnet_join_tun tun_get_user result=$retval:s64',
        'ip_entry':'p:kprobes/mnet_join_ip ip_rcv skb=%di sport=+20(+'+str(offsets['data'])+'(%di)):u16 dport=+22(+'+str(offsets['data'])+'(%di)):u16',
    }
    report['probes']={}
    names={}
    for label,spec in specs.items():
        name=spec.split()[0].split('/')[-1];names[label]=name
        registered=api.shell('sh','-c','printf "%s\\n" "$1" >> "$2"','probe',spec,trace+'/kprobe_events').returncode==0
        enabled=False
        if registered:
            hooks.append((name,trace));enabled=api.shell('sh','-c','printf 1 > "$1"','probe',instance+'/events/kprobes/'+name+'/enable').returncode==0
        report['probes'][label]={'registered':registered,'enabled':enabled}
    # Existing capability hook events are cleared; only this interval is parsed.
    api.shell('sh','-c',': > "$1"','probe',instance+'/trace')
    api.shell('sh','-c','printf 1 > "$1"','probe',instance+'/tracing_on')
    synthetic=api.shell('/data/local/tmp/mnet-correlation',timeout=15)
    api.shell('sh','-c','printf 0 > "$1"','probe',instance+'/tracing_on')
    raw=api.text(api.shell('cat',instance+'/trace'))
    try:row=json.loads(api.text(synthetic))
    except ValueError:report['helper_result_unavailable']=True;return report
    report['helper']={k:v for k,v in row.items() if k in ('udp_sent','udp_received','mark_read','tun_created','tun_configured','tun_valid_written','tun_valid_received','tun_corrupt_written','tun_corrupt_received','tun_corrupt_timeout') and type(v) in (bool,int)}
    # All task, pointer and port joins are done here. No raw identifiers exported.
    pid=row['pid'];events=[]
    for line in raw.splitlines():
        match=re.search(r'-(\d+)\s+\[\d+\].*?\s(\d+\.\d+):\s+(\w+):\s+(.*)',line)
        if not match:continue
        task,ts,event,body=match.groups()
        fields=dict(re.findall(r'(\w+)=(0x[0-9a-fA-F]+|-?\d+|[0-9a-fA-F]+)(?=\s|$)',body))
        values={k:int(v,16 if v.startswith('0x') or k in ('skbaddr','skb','sk') else 10) for k,v in fields.items() if k not in ('reason',)}
        events.append((int(task),float(ts),event,values,body))
    def port_net(n):return ((n&255)<<8)|((n>>8)&255)
    entries=[x for x in events if x[0]==pid and x[2]==names['udp_entry'] and x[3].get('sport')==row.get('udp_source')]
    returns=[x for x in events if x[0]==pid and x[2]==names['udp_return']]
    security=[x for x in events if x[0]==pid and x[2]==names['security_return']]
    # sendto() on an unconnected UDP socket doesn't necessarily store dport in sk.
    report['socket_join']={'same_pid_and_source_port':len(entries)==1,'mark_matches_getsockopt':len(entries)==1 and entries[0][3].get('mark')==row.get('mark'),'source_mark_read':row.get('mark_read') is True,'send_return_matches':any(x[3].get('result')==row.get('udp_sent') for x in returns),'security_return_zero_same_pid':any(x[3].get('result')==0 for x in security),'entry_before_return':bool(entries and returns and entries[0][1]<=returns[-1][1])}
    tun_returns=[x for x in events if x[0]==pid and x[2]==names['tun_return']]
    packets=[x for x in events if x[2]==names['ip_entry'] and port_net(x[3].get('sport',0))==45997 and port_net(x[3].get('dport',0))==row.get('tun_dest')]
    drop_events=[x for x in events if x[2]=='kfree_skb']
    joined=[]
    for index,packet in enumerate(packets):
        end=packets[index+1][1] if index+1<len(packets) else float('inf')
        pointer=packet[3].get('skb')
        drops=[x for x in drop_events if packet[1]<=x[1]<end and x[3].get('skbaddr')==pointer]
        joined.append({'input_writer_pid_matches':packet[0]==pid,'matched_drop':bool(drops),'drop_reason_categories':[re.search(r'reason=([A-Z_]+)',x[4]).group(1) for x in drops if re.search(r'reason=([A-Z_]+)',x[4])]})
    report['tun_join']={'same_pid_write_return_count':sum(x[3].get('result')==35 for x in tun_returns),'matched_ip_tuple_count':len(packets),'packet_drop_joins':joined,'valid_received_corrupt_not_received':row.get('tun_valid_received')==7 and row.get('tun_corrupt_received')==-1 and row.get('tun_corrupt_timeout') is True}
    stats=api.text(api.shell('cat',instance+'/per_cpu/cpu0/stats'))
    report['trace_overrun_cpu0_zero']=bool(re.search(r'^overrun:\s+0$',stats,re.M))
    return report
